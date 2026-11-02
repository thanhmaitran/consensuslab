package sim

import (
	"testing"
	"time"

	"github.com/thanhmaitran/consensuslab/internal/network"
)

func testConfig(nodes int, seed int64) Config {
	c := DefaultConfig()
	c.NodeCount = nodes
	c.Seed = seed
	c.ScenarioID = "test"
	return c
}

func TestEventQueueOrdersByTimeThenSequence(t *testing.T) {
	q := NewEventQueue()
	q.Schedule(Event{At: 20, Seq: 1})
	q.Schedule(Event{At: 10, Seq: 3})
	q.Schedule(Event{At: 10, Seq: 2})
	var got []uint64
	for {
		ev, ok := q.Next()
		if !ok {
			break
		}
		got = append(got, ev.Seq)
	}
	want := []uint64{2, 3, 1}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order = %v, want %v", got, want)
		}
	}
}

func TestConfigValidation(t *testing.T) {
	c := testConfig(5, 1)
	c.NodeCount = 8
	if c.Validate() == nil {
		t.Error("8 nodes accepted")
	}
	c = testConfig(5, 1)
	c.Network.LossProbability = 2
	if c.Validate() == nil {
		t.Error("loss probability 2 accepted")
	}
	c = testConfig(5, 1)
	c.ElectionTimeoutMin = c.HeartbeatInterval
	if c.Validate() == nil {
		t.Error("election timeout <= heartbeat accepted")
	}
}

func TestElectsExactlyOneLeader(t *testing.T) {
	e, err := NewEngine(testConfig(3, 1))
	if err != nil {
		t.Fatal(err)
	}
	e.RunFor(3 * time.Second)
	leaders := 0
	for _, id := range e.NodeIDs() {
		if e.Nodes[id].Status().Role == "leader" {
			leaders++
		}
	}
	if leaders != 1 {
		t.Fatalf("leaders = %d, want 1", leaders)
	}
	if r := e.Finish(); r.Status != "passed" {
		t.Fatalf("verifier: %+v", r.Violations)
	}
}

func TestSameSeedSameTrace(t *testing.T) {
	run := func(seed int64) string {
		e, err := NewEngine(testConfig(5, seed))
		if err != nil {
			t.Fatal(err)
		}
		_, _ = e.ScheduleCommand(2*time.Second, "", "SET a 1")
		_ = e.ScheduleCrash(3*time.Second, "", time.Second)
		e.RunUntil(8 * time.Second)
		return e.Recorder.TraceHash()
	}
	if run(7) != run(7) {
		t.Fatal("identical configuration produced different traces")
	}
	if run(7) == run(8) {
		t.Fatal("different seeds produced the same trace")
	}
}

func TestCrashedNodeStaysSilent(t *testing.T) {
	e, _ := NewEngine(testConfig(3, 1))
	if err := e.Crash("node-1"); err != nil {
		t.Fatal(err)
	}
	e.RunFor(2 * time.Second)
	if !e.IsDown("node-1") {
		t.Fatal("node-1 should be down")
	}
	if st := e.Nodes["node-1"].Status(); st.CurrentTerm != 0 {
		t.Fatalf("crashed node kept running: %+v", st)
	}
	if e.Leader() == "" {
		t.Fatal("remaining quorum should elect a leader")
	}
}

func TestCommandCommits(t *testing.T) {
	e, _ := NewEngine(testConfig(3, 2))
	e.RunFor(2 * time.Second)
	if _, err := e.Submit("", "SET a 1"); err != nil {
		t.Fatal(err)
	}
	e.RunFor(time.Second)
	m := e.Metrics()
	if m.CommandsCommitted != 1 || len(m.CommitLatencies) != 1 {
		t.Fatalf("metrics = %+v", m)
	}
}

func TestUnknownNodeAndInvalidCommand(t *testing.T) {
	e, _ := NewEngine(testConfig(3, 1))
	if err := e.Crash("node-9"); err == nil {
		t.Error("unknown node accepted")
	}
	if _, err := e.Submit("", "BOGUS"); err == nil {
		t.Error("invalid command accepted")
	}
	if err := e.SchedulePartitionSizes(0, []int{1, 1}, false); err == nil {
		t.Error("partition sizes not summing to the node count accepted")
	}
}

func TestPartitionSizesPlaceLeaderInLastGroup(t *testing.T) {
	e, _ := NewEngine(testConfig(5, 3))
	e.RunFor(3 * time.Second)
	leader := e.Leader()
	if leader == "" {
		t.Fatal("no leader")
	}
	if err := e.SchedulePartitionSizes(e.Now, []int{3, 2}, true); err != nil {
		t.Fatal(err)
	}
	e.ProcessDue()
	groups := e.Network.Groups()
	if len(groups) != 2 || len(groups[1]) != 2 {
		t.Fatalf("groups = %v", groups)
	}
	found := false
	for _, id := range groups[1] {
		if id == leader {
			found = true
		}
	}
	if !found {
		t.Fatalf("leader %s not in the last group %v", leader, groups)
	}
}

func TestNetworkChangeApplies(t *testing.T) {
	e, _ := NewEngine(testConfig(3, 1))
	if err := e.SetNetwork(network.FromMillis(50, 0, 0)); err != nil {
		t.Fatal(err)
	}
	if e.Network.Config().BaseDelay != 50*time.Millisecond {
		t.Fatal("network config not applied")
	}
	if err := e.SetNetwork(network.Config{LossProbability: 3}); err == nil {
		t.Fatal("invalid network config accepted")
	}
}
