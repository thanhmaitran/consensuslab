// Package network models message delivery between simulated nodes: delay,
// jitter, random loss and partitions. It never touches Raft state.
package network

import (
	"errors"
	"fmt"
	"math/rand"
	"time"

	"github.com/thanhmaitran/consensuslab/internal/raft"
)

// Transport is the only way a node's outgoing messages leave the node.
// The simulation engine implements it so that it controls delivery.
type Transport interface {
	Send(msg raft.Message)
}

// Config describes link behaviour that applies to every message.
type Config struct {
	BaseDelay       time.Duration
	Jitter          time.Duration
	LossProbability float64
}

// FromMillis builds a Config from millisecond values.
func FromMillis(baseDelayMs, jitterMs, loss float64) Config {
	return Config{
		BaseDelay:       time.Duration(baseDelayMs * float64(time.Millisecond)),
		Jitter:          time.Duration(jitterMs * float64(time.Millisecond)),
		LossProbability: loss,
	}
}

// Validate rejects inconsistent configurations.
func (c Config) Validate() error {
	if c.BaseDelay < 0 || c.Jitter < 0 {
		return errors.New("network: delay and jitter must not be negative")
	}
	if c.LossProbability < 0 || c.LossProbability > 1 {
		return fmt.Errorf("network: loss probability %v is outside [0, 1]", c.LossProbability)
	}
	return nil
}

// Decision is the outcome of routing one message.
type Decision struct {
	Deliver bool
	Delay   time.Duration
	Reason  string // set when Deliver is false
}

// Stats counts message outcomes.
type Stats struct {
	Sent             uint64 `json:"sent"`
	Delivered        uint64 `json:"delivered"`
	DroppedLoss      uint64 `json:"droppedLoss"`
	DroppedPartition uint64 `json:"droppedPartition"`
	DroppedDown      uint64 `json:"droppedDown"`
}

// DeliveryRate returns Delivered/Sent and false when nothing was sent.
func (s Stats) DeliveryRate() (float64, bool) {
	if s.Sent == 0 {
		return 0, false
	}
	return float64(s.Delivered) / float64(s.Sent), true
}

// Network decides what happens to each message. It owns its own seeded RNG.
type Network struct {
	cfg    Config
	rng    *rand.Rand
	group  map[raft.NodeID]int
	groups [][]raft.NodeID
	stats  Stats
}

// New creates a network. The seed makes loss and jitter reproducible.
func New(cfg Config, seed int64) (*Network, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &Network{
		cfg:   cfg,
		rng:   rand.New(rand.NewSource(seed)),
		group: make(map[raft.NodeID]int),
	}, nil
}

func (n *Network) Config() Config { return n.cfg }

func (n *Network) SetConfig(cfg Config) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	n.cfg = cfg
	return nil
}

// SetPartition splits the cluster into the given groups. Nodes can talk only
// within their own group; nodes not listed form one additional group.
func (n *Network) SetPartition(groups [][]raft.NodeID) error {
	assigned := make(map[raft.NodeID]int)
	stored := make([][]raft.NodeID, 0, len(groups))
	for i, g := range groups {
		if len(g) == 0 {
			return errors.New("network: partition groups must not be empty")
		}
		for _, id := range g {
			if _, dup := assigned[id]; dup {
				return fmt.Errorf("network: node %q appears in more than one group", id)
			}
			assigned[id] = i + 1
		}
		stored = append(stored, append([]raft.NodeID(nil), g...))
	}
	n.group = assigned
	n.groups = stored
	return nil
}

// Heal removes any partition.
func (n *Network) Heal() {
	n.group = make(map[raft.NodeID]int)
	n.groups = nil
}

func (n *Network) Partitioned() bool { return len(n.groups) > 0 }

// Groups returns a copy of the explicit partition groups.
func (n *Network) Groups() [][]raft.NodeID {
	out := make([][]raft.NodeID, len(n.groups))
	for i, g := range n.groups {
		out[i] = append([]raft.NodeID(nil), g...)
	}
	return out
}

// Reachable reports whether a and b are currently allowed to communicate.
func (n *Network) Reachable(a, b raft.NodeID) bool {
	return n.group[a] == n.group[b]
}

// Route decides the fate of one message and updates the statistics.
func (n *Network) Route(from, to raft.NodeID) Decision {
	n.stats.Sent++
	if !n.Reachable(from, to) {
		n.stats.DroppedPartition++
		return Decision{Reason: "partition"}
	}
	if n.cfg.LossProbability > 0 && n.rng.Float64() < n.cfg.LossProbability {
		n.stats.DroppedLoss++
		return Decision{Reason: "loss"}
	}
	delay := n.cfg.BaseDelay
	if n.cfg.Jitter > 0 {
		delay += time.Duration(n.rng.Int63n(int64(n.cfg.Jitter) + 1))
	}
	return Decision{Deliver: true, Delay: delay}
}

// RecordDelivered, RecordDroppedPartition and RecordDroppedDown are called by
// the engine when a routed message reaches (or fails to reach) its target.
func (n *Network) RecordDelivered()        { n.stats.Delivered++ }
func (n *Network) RecordDroppedPartition() { n.stats.DroppedPartition++ }
func (n *Network) RecordDroppedDown()      { n.stats.DroppedDown++ }

func (n *Network) Stats() Stats { return n.stats }
