package sim

import (
	"errors"
	"fmt"
	"time"

	"github.com/thanhmaitran/consensuslab/internal/network"
	"github.com/thanhmaitran/consensuslab/internal/raft"
)

// MaxNodes bounds the cluster size.
const MaxNodes = 7

// Config describes one simulation.
type Config struct {
	ScenarioID string
	Seed       int64
	NodeCount  int
	Network    network.Config

	ElectionTimeoutMin time.Duration
	ElectionTimeoutMax time.Duration
	HeartbeatInterval  time.Duration
	TickInterval       time.Duration

	// MaxVirtualTime stops the engine: events after it are never processed.
	MaxVirtualTime time.Duration
	// MaxEvents bounds the in-memory event history.
	MaxEvents int
	// VerboseEvents records heartbeats and every message outcome.
	VerboseEvents bool
}

// DefaultConfig returns sensible defaults for a 5-node cluster.
func DefaultConfig() Config {
	return Config{
		ScenarioID:         "interactive",
		Seed:               1,
		NodeCount:          5,
		Network:            network.FromMillis(10, 5, 0),
		ElectionTimeoutMin: 150 * time.Millisecond,
		ElectionTimeoutMax: 300 * time.Millisecond,
		HeartbeatInterval:  50 * time.Millisecond,
		TickInterval:       10 * time.Millisecond,
		MaxVirtualTime:     10 * time.Minute,
		MaxEvents:          100000,
	}
}

// Validate rejects inconsistent configurations.
func (c Config) Validate() error {
	if c.NodeCount < 1 || c.NodeCount > MaxNodes {
		return fmt.Errorf("sim: node count %d is outside [1, %d]", c.NodeCount, MaxNodes)
	}
	if err := c.Network.Validate(); err != nil {
		return err
	}
	if c.TickInterval <= 0 {
		return errors.New("sim: tick interval must be positive")
	}
	if time.Duration(c.NodeCount)*time.Millisecond > c.TickInterval {
		return errors.New("sim: tick interval too small for the start-up stagger")
	}
	if c.HeartbeatInterval < c.TickInterval {
		return errors.New("sim: heartbeat interval must not be smaller than the tick interval")
	}
	if c.ElectionTimeoutMin <= c.HeartbeatInterval || c.ElectionTimeoutMax < c.ElectionTimeoutMin {
		return errors.New("sim: election timeout must exceed the heartbeat interval and max must be >= min")
	}
	if c.MaxVirtualTime <= 0 {
		return errors.New("sim: max virtual time must be positive")
	}
	return nil
}

// NodeName returns the canonical ID of the i-th node (1-based).
func NodeName(i int) raft.NodeID {
	return raft.NodeID(fmt.Sprintf("node-%d", i))
}
