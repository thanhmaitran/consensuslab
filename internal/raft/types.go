package raft

import "errors"

// NodeID identifies a node inside a cluster.
type NodeID string

// Role is the current role of a node.
type Role uint8

const (
	Follower Role = iota
	Candidate
	Leader
)

func (r Role) String() string {
	switch r {
	case Follower:
		return "follower"
	case Candidate:
		return "candidate"
	case Leader:
		return "leader"
	default:
		return "unknown"
	}
}

// LogEntry is one command stored in the replicated log.
// Index 0 is a sentinel entry (term 0) that is never applied.
type LogEntry struct {
	Index   uint64 `json:"index"`
	Term    uint64 `json:"term"`
	Command string `json:"command"`
}

// NodeStatus is a read-only observation DTO. LogLength counts real
// entries (the sentinel is excluded), so the last log index equals LogLength.
type NodeStatus struct {
	ID          NodeID `json:"id"`
	Role        string `json:"role"`
	CurrentTerm uint64 `json:"currentTerm"`
	CommitIndex uint64 `json:"commitIndex"`
	LastApplied uint64 `json:"lastApplied"`
	LogLength   int    `json:"logLength"`
	LeaderID    NodeID `json:"leaderId"`
	VotedFor    NodeID `json:"votedFor"`
}

// NodeSnapshot is a copy of the state needed by the verifier.
// Log includes the sentinel entry at index 0.
type NodeSnapshot struct {
	Status NodeStatus
	Log    []LogEntry
}

var (
	// ErrNotLeader is returned by Propose on a node that is not the leader.
	ErrNotLeader = errors.New("raft: node is not the leader")
	// ErrInvalidCommand is returned for commands that fail validation.
	ErrInvalidCommand = errors.New("raft: invalid command")
)
