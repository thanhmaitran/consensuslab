package tests

import (
	"fmt"
	"testing"
	"time"

	"github.com/thanhmaitran/consensuslab/internal/sim"
)

// INT-01: a 3-node cluster elects exactly one leader.
func TestINT01_ClusterElectsLeader(t *testing.T) {
	e := newEngine(t, 3, 1, nil)
	leader := runUntilStable(t, e, 5*time.Second)
	count := 0
	for _, id := range e.NodeIDs() {
		if e.Nodes[id].Status().Role == "leader" {
			count++
		}
	}
	if leader == "" || count != 1 {
		t.Fatalf("leader=%q leaders=%d", leader, count)
	}
	mustPass(t, e)
}

// INT-02: a command submitted to the leader is committed everywhere.
func TestINT02_CommandCommits(t *testing.T) {
	e := newEngine(t, 3, 2, nil)
	runUntilStable(t, e, 5*time.Second)
	if _, err := e.Submit("", "SET greeting hello"); err != nil {
		t.Fatal(err)
	}
	e.RunFor(time.Second)
	if got := e.Metrics().CommandsCommitted; got != 1 {
		t.Fatalf("committed = %d, want 1", got)
	}
	if !converged(e) {
		t.Fatal("nodes did not converge")
	}
	mustPass(t, e)
}

// INT-03: after the leader crashes the remaining quorum elects a new leader.
func TestINT03_LeaderCrashFailover(t *testing.T) {
	e := newEngine(t, 5, 3, nil)
	old := runUntilStable(t, e, 5*time.Second)
	oldTerm := e.Nodes[old].Status().CurrentTerm
	if err := e.Crash(old); err != nil {
		t.Fatal(err)
	}
	e.RunFor(3 * time.Second)
	nl := e.Leader()
	if nl == "" || nl == old {
		t.Fatalf("no new leader (old=%s new=%q)", old, nl)
	}
	if e.Nodes[nl].Status().CurrentTerm <= oldTerm {
		t.Fatal("new leader does not have a higher term")
	}
	if _, err := e.Submit("", "SET after failover"); err != nil {
		t.Fatal(err)
	}
	e.RunFor(time.Second)
	if e.Metrics().CommandsCommitted != 1 {
		t.Fatal("new leader could not commit")
	}
	mustPass(t, e)
}

// INT-04: with one follower down a 3-node cluster keeps committing.
func TestINT04_FollowerCrashKeepsQuorum(t *testing.T) {
	e := newEngine(t, 3, 4, nil)
	runUntilStable(t, e, 5*time.Second)
	if err := e.Crash(firstFollower(e)); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Submit("", "SET x 1"); err != nil {
		t.Fatal(err)
	}
	e.RunFor(time.Second)
	if e.Metrics().CommandsCommitted != 1 {
		t.Fatal("command not committed with a quorum of 2/3")
	}
	mustPass(t, e)
}

// INT-05: a restarted follower catches up.
func TestINT05_FollowerRestartCatchesUp(t *testing.T) {
	e := newEngine(t, 3, 5, nil)
	runUntilStable(t, e, 5*time.Second)
	f := firstFollower(e)
	if err := e.Crash(f); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		if _, err := e.Submit("", fmt.Sprintf("SET k%d v%d", i, i)); err != nil {
			t.Fatal(err)
		}
		e.RunFor(100 * time.Millisecond)
	}
	e.RunFor(500 * time.Millisecond)
	if err := e.Restart(f); err != nil {
		t.Fatal(err)
	}
	e.RunFor(3 * time.Second)
	if !converged(e) {
		t.Fatal("restarted follower did not catch up")
	}
	mustPass(t, e)
}

// INT-06: large delays never break safety.
func TestINT06_HighLatencySafety(t *testing.T) {
	e := newEngine(t, 5, 6, func(c *sim.Config) {
		c.Network = netMillis(100, 50, 0)
		c.HeartbeatInterval = 150 * time.Millisecond
		c.ElectionTimeoutMin = 600 * time.Millisecond
		c.ElectionTimeoutMax = 1200 * time.Millisecond
	})
	for i := 0; i < 10; i++ {
		_, _ = e.ScheduleCommand(time.Duration(3000+i*500)*time.Millisecond, "", fmt.Sprintf("SET k%d v", i))
	}
	e.RunUntil(20 * time.Second)
	mustPass(t, e)
}

// INT-07: message loss never breaks safety and replication is retried.
func TestINT07_MessageLoss(t *testing.T) {
	e := newEngine(t, 5, 7, func(c *sim.Config) { c.Network = netMillis(10, 5, 0.2) })
	for i := 0; i < 20; i++ {
		_, _ = e.ScheduleCommand(time.Duration(2000+i*500)*time.Millisecond, "", fmt.Sprintf("SET k%d v", i))
	}
	e.RunUntil(30 * time.Second)
	if e.Metrics().CommandsCommitted == 0 {
		t.Fatal("nothing committed under 20% loss")
	}
	mustPass(t, e)
}

// INT-08, INT-10, INT-11: a 3/2 partition with the leader in the minority.
func TestINT08_10_11_PartitionConflictAndHeal(t *testing.T) {
	e := newEngine(t, 5, 11, nil)
	old := runUntilStable(t, e, 5*time.Second)
	if err := e.SchedulePartitionSizes(e.Now, []int{3, 2}, true); err != nil {
		t.Fatal(err)
	}
	e.ProcessDue()

	before := e.Nodes[old].Status().CommitIndex
	if _, err := e.Submit(old, "SET minority 1"); err != nil {
		t.Fatal(err)
	}
	e.RunFor(3 * time.Second)
	if e.Nodes[old].Status().CommitIndex != before {
		t.Fatal("isolated minority leader committed an entry")
	}
	nl := e.Leader()
	if nl == "" || nl == old {
		t.Fatalf("majority side has no new leader (old=%s, leader=%q)", old, nl)
	}
	majorityBefore := e.Nodes[nl].Status().CommitIndex
	if _, err := e.Submit(nl, "SET majority 1"); err != nil {
		t.Fatal(err)
	}
	e.RunFor(time.Second)
	if e.Nodes[nl].Status().CommitIndex <= majorityBefore {
		t.Fatal("majority side could not commit")
	}

	e.Heal()
	e.RunFor(3 * time.Second)
	if !converged(e) {
		t.Fatal("logs did not converge after healing")
	}
	if hasCommand(e, old, "SET minority 1") {
		t.Fatal("the uncommitted minority entry survived the heal")
	}
	if !hasCommand(e, old, "SET majority 1") {
		t.Fatal("the old leader lacks the entry committed by the majority")
	}
	mustPass(t, e)
}

// INT-08b: with no quorum anywhere nothing commits, and safety still holds.
func TestINT08b_NoQuorumNoProgress(t *testing.T) {
	e := newEngine(t, 5, 12, nil)
	runUntilStable(t, e, 5*time.Second)
	if err := e.SchedulePartitionSizes(e.Now, []int{2, 2, 1}, false); err != nil {
		t.Fatal(err)
	}
	e.ProcessDue()
	e.RunFor(time.Second) // let stale leader state settle
	committed := e.Metrics().CommandsCommitted
	for _, id := range e.NodeIDs() {
		_, _ = e.Submit(id, "SET nope 1")
	}
	e.RunFor(3 * time.Second)
	if e.Metrics().CommandsCommitted != committed {
		t.Fatal("a command committed although no group has a quorum")
	}
	e.Heal()
	e.RunFor(4 * time.Second)
	if !converged(e) {
		t.Fatal("no convergence after healing")
	}
	mustPass(t, e)
}

// INT-09: simultaneous candidates never produce two leaders in one term.
func TestINT09_CompetingCandidates(t *testing.T) {
	e := newEngine(t, 3, 9, func(c *sim.Config) {
		c.ElectionTimeoutMin = 200 * time.Millisecond
		c.ElectionTimeoutMax = 200 * time.Millisecond
	})
	e.RunUntil(5 * time.Second)
	if e.Metrics().ElectionsStarted < 3 {
		t.Fatalf("expected contention, only %d elections started", e.Metrics().ElectionsStarted)
	}
	mustPass(t, e)
}

// INT-12: same seed and inputs give the same trace.
func TestINT12_ReplayIsDeterministic(t *testing.T) {
	run := func() string {
		e := newEngine(t, 5, 99, func(c *sim.Config) { c.Network = netMillis(10, 5, 0.05) })
		for i := 0; i < 10; i++ {
			_, _ = e.ScheduleCommand(time.Duration(2000+i*200)*time.Millisecond, "", fmt.Sprintf("SET k%d v", i))
		}
		_ = e.ScheduleCrash(3*time.Second, "", time.Second)
		_ = e.SchedulePartitionSizes(5*time.Second, []int{3, 2}, true)
		e.ScheduleHeal(7 * time.Second)
		e.RunUntil(10 * time.Second)
		return e.Recorder.TraceHash()
	}
	if a, b := run(), run(); a != b {
		t.Fatalf("traces differ: %s vs %s", a, b)
	}
}
