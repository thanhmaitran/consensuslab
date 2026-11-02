package verify

import (
	"testing"
	"time"

	"github.com/thanhmaitran/consensuslab/internal/raft"
)

type fakeNode struct {
	snap   raft.NodeSnapshot
	digest string
}

func (f *fakeNode) ID() raft.NodeID                             { return f.snap.Status.ID }
func (f *fakeNode) Tick()                                       {}
func (f *fakeNode) HandleMessage(raft.Message)                  {}
func (f *fakeNode) Status() raft.NodeStatus                     { return f.snap.Status }
func (f *fakeNode) Propose(string) (uint64, uint64, error)      { return 0, 0, raft.ErrNotLeader }
func (f *fakeNode) Restart()                                    {}
func (f *fakeNode) Snapshot() raft.NodeSnapshot                 { return f.snap }
func (f *fakeNode) StateDigest() string                         { return f.digest }
func (f *fakeNode) CommittedEntry(uint64) (raft.LogEntry, bool) { return raft.LogEntry{}, false }

func node(id string, role string, term, commit uint64, entries ...raft.LogEntry) *fakeNode {
	log := append([]raft.LogEntry{{}}, entries...)
	return &fakeNode{snap: raft.NodeSnapshot{
		Status: raft.NodeStatus{
			ID: raft.NodeID(id), Role: role, CurrentTerm: term,
			CommitIndex: commit, LastApplied: commit, LogLength: len(entries),
		},
		Log: log,
	}}
}

func le(i, term uint64, cmd string) raft.LogEntry {
	return raft.LogEntry{Index: i, Term: term, Command: cmd}
}

func invariants(v *Verifier) map[string]int {
	out := map[string]int{}
	for _, x := range v.Report().Violations {
		out[x.Invariant]++
	}
	return out
}

func TestCleanRunPasses(t *testing.T) {
	v := New("s", 1)
	a := node("a", "leader", 1, 1, le(1, 1, "SET x 1"))
	b := node("b", "follower", 1, 1, le(1, 1, "SET x 1"))
	v.ObserveNode(time.Second, a)
	v.ObserveNode(time.Second, b)
	v.PeriodicCheck(time.Second, []raft.Node{a, b})
	if r := v.Report(); r.Status != "passed" || r.InvariantsChecked != 5 {
		t.Fatalf("report = %+v", r)
	}
}

func TestTwoLeadersInOneTerm(t *testing.T) {
	v := New("s", 1)
	v.ObserveNode(0, node("a", "leader", 3, 0))
	v.ObserveNode(0, node("b", "leader", 3, 0))
	if invariants(v)[ElectionSafety] != 1 {
		t.Fatalf("violations = %v", invariants(v))
	}
}

func TestDivergentCommittedEntries(t *testing.T) {
	v := New("s", 1)
	v.ObserveNode(0, node("a", "follower", 1, 1, le(1, 1, "SET x 1")))
	v.ObserveNode(0, node("b", "follower", 1, 1, le(1, 1, "SET x 2")))
	if invariants(v)[StateMachineSafety] != 1 {
		t.Fatalf("violations = %v", invariants(v))
	}
}

func TestLeaderMissingCommittedEntry(t *testing.T) {
	v := New("s", 1)
	v.ObserveNode(0, node("a", "follower", 1, 1, le(1, 1, "SET x 1")))
	v.ObserveNode(0, node("b", "leader", 2, 0)) // later-term leader with an empty log
	if invariants(v)[LeaderCompleteness] != 1 {
		t.Fatalf("violations = %v", invariants(v))
	}
}

func TestLeaderOfSameTermNeedNotHaveEarlierCommit(t *testing.T) {
	v := New("s", 1)
	v.ObserveNode(0, node("a", "follower", 2, 1, le(1, 2, "SET x 1")))
	v.ObserveNode(0, node("b", "leader", 2, 0))
	if n := invariants(v)[LeaderCompleteness]; n != 0 {
		t.Fatalf("false positive: %d", n)
	}
}

func TestLogMatchingViolation(t *testing.T) {
	v := New("s", 1)
	a := node("a", "follower", 2, 0, le(1, 1, "SET x 1"), le(2, 2, "SET y 1"))
	b := node("b", "follower", 2, 0, le(1, 1, "SET x 2"), le(2, 2, "SET y 1"))
	v.PeriodicCheck(0, []raft.Node{a, b})
	if invariants(v)[LogMatching] != 1 {
		t.Fatalf("violations = %v", invariants(v))
	}
}

func TestIndexBounds(t *testing.T) {
	v := New("s", 1)
	n := node("a", "follower", 1, 5, le(1, 1, "SET x 1"))
	v.ObserveNode(0, n)
	if invariants(v)[IndexBounds] == 0 {
		t.Fatal("commitIndex beyond the log was not reported")
	}
}

func TestDifferentStatesAtSameAppliedIndex(t *testing.T) {
	v := New("s", 1)
	a := node("a", "follower", 1, 1, le(1, 1, "SET x 1"))
	b := node("b", "follower", 1, 1, le(1, 1, "SET x 1"))
	a.digest, b.digest = "aaa", "bbb"
	v.PeriodicCheck(0, []raft.Node{a, b})
	if invariants(v)[StateMachineSafety] != 1 {
		t.Fatalf("violations = %v", invariants(v))
	}
}
