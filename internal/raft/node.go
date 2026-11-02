package raft

import (
	"errors"
	"fmt"
	"time"
)

// Node is the surface the simulation engine and the verifier use.
// The engine drives time through Tick and delivers messages through
// HandleMessage; nodes never touch sockets or timers themselves.
type Node interface {
	ID() NodeID
	Tick()
	HandleMessage(Message)
	Status() NodeStatus

	// Propose appends a client command if the node is the leader.
	Propose(command string) (index, term uint64, err error)
	// Restart simulates a process restart: persistent state (term, vote, log)
	// survives, volatile state (role, commit index, state machine) is reset.
	Restart()
	// Snapshot returns a copy of the log and status for verification.
	Snapshot() NodeSnapshot
	// CommittedEntry returns the entry at index if it is committed on this node.
	CommittedEntry(index uint64) (LogEntry, bool)
	// StateDigest fingerprints the applied state machine.
	StateDigest() string
}

// Config configures a Server.
type Config struct {
	ID    NodeID
	Peers []NodeID // all cluster members, including ID

	// Seed feeds the node's private RNG used for election timeouts.
	Seed int64

	ElectionTimeoutMin time.Duration
	ElectionTimeoutMax time.Duration
	HeartbeatInterval  time.Duration

	// VerboseEvents also emits one HeartbeatSent event per heartbeat round.
	VerboseEvents bool
}

// Validate checks the configuration for consistency.
func (c Config) Validate() error {
	if c.ID == "" {
		return errors.New("raft: node ID must not be empty")
	}
	seen := make(map[NodeID]bool, len(c.Peers))
	found := false
	for _, p := range c.Peers {
		if p == "" {
			return errors.New("raft: peer ID must not be empty")
		}
		if seen[p] {
			return fmt.Errorf("raft: duplicate peer ID %q", p)
		}
		seen[p] = true
		if p == c.ID {
			found = true
		}
	}
	if !found {
		return fmt.Errorf("raft: peers must include the node itself (%q)", c.ID)
	}
	if c.ElectionTimeoutMin <= 0 || c.ElectionTimeoutMax < c.ElectionTimeoutMin {
		return errors.New("raft: invalid election timeout range")
	}
	if c.HeartbeatInterval <= 0 || c.HeartbeatInterval >= c.ElectionTimeoutMin {
		return errors.New("raft: heartbeat interval must be positive and smaller than the minimum election timeout")
	}
	return nil
}

// Env connects a Server to its environment. Now and Send are required.
type Env struct {
	// Now returns the current (virtual) time.
	Now func() time.Duration
	// Send hands a message to the transport.
	Send func(Message)
	// Emit publishes a structured observation. Optional.
	Emit func(eventType string, term, logIndex uint64, metadata map[string]any)
}
