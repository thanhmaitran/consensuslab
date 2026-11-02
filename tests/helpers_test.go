package tests

import (
	"math/rand"
	"testing"
	"time"

	"github.com/thanhmaitran/consensuslab/internal/network"
	"github.com/thanhmaitran/consensuslab/internal/raft"
	"github.com/thanhmaitran/consensuslab/internal/sim"
)

func newEngine(t *testing.T, nodes int, seed int64, mutate func(*sim.Config)) *sim.Engine {
	t.Helper()
	cfg := sim.DefaultConfig()
	cfg.ScenarioID = t.Name()
	cfg.NodeCount = nodes
	cfg.Seed = seed
	if mutate != nil {
		mutate(&cfg)
	}
	e, err := sim.NewEngine(cfg)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	return e
}

// runUntilStable advances time in 20 ms steps until the cluster is stable.
func runUntilStable(t *testing.T, e *sim.Engine, limit time.Duration) raft.NodeID {
	t.Helper()
	deadline := e.Now + limit
	for e.Now < deadline {
		e.RunFor(20 * time.Millisecond)
		if e.ClusterStable() {
			return e.Leader()
		}
	}
	t.Fatalf("cluster not stable after %v (seed %d)", limit, e.Seed)
	return ""
}

// mustPass fails the test, printing seed and violations, if any invariant broke.
func mustPass(t *testing.T, e *sim.Engine) {
	t.Helper()
	r := e.Finish()
	if r.Status != "passed" {
		t.Fatalf("seed %d: %d safety violations, first: %+v\ninputs: %+v", e.Seed, len(r.Violations), r.Violations[0], e.Inputs())
	}
}

// converged: every node holds the same log, commit index and state.
func converged(e *sim.Engine) bool {
	var ref raft.NodeSnapshot
	var refDigest string
	for i, id := range e.NodeIDs() {
		n := e.Nodes[id]
		snap := n.Snapshot()
		if snap.Status.CommitIndex != uint64(snap.Status.LogLength) {
			return false
		}
		if i == 0 {
			ref, refDigest = snap, n.StateDigest()
			continue
		}
		if len(snap.Log) != len(ref.Log) || snap.Status.CommitIndex != ref.Status.CommitIndex || n.StateDigest() != refDigest {
			return false
		}
		for j := range snap.Log {
			if snap.Log[j] != ref.Log[j] {
				return false
			}
		}
	}
	return true
}

func hasCommand(e *sim.Engine, id raft.NodeID, cmd string) bool {
	for _, entry := range e.Nodes[id].Snapshot().Log {
		if entry.Command == cmd {
			return true
		}
	}
	return false
}

func firstFollower(e *sim.Engine) raft.NodeID {
	leader := e.Leader()
	for _, id := range e.NodeIDs() {
		if id != leader {
			return id
		}
	}
	return ""
}

func randomSizes(rng *rand.Rand, n int) []int {
	groups := 2 + rng.Intn(2)
	if n < groups {
		groups = 2
	}
	sizes := make([]int, groups)
	for i := range sizes {
		sizes[i] = 1
	}
	for left := n - groups; left > 0; left-- {
		sizes[rng.Intn(groups)]++
	}
	return sizes
}

func netMillis(base, jitter, loss float64) network.Config {
	return network.FromMillis(base, jitter, loss)
}
