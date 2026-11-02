package network

import (
	"testing"
	"time"

	"github.com/thanhmaitran/consensuslab/internal/raft"
)

func TestValidate(t *testing.T) {
	if (Config{LossProbability: 1.5}).Validate() == nil {
		t.Error("loss > 1 accepted")
	}
	if (Config{LossProbability: -0.1}).Validate() == nil {
		t.Error("negative loss accepted")
	}
	if (Config{BaseDelay: -time.Millisecond}).Validate() == nil {
		t.Error("negative delay accepted")
	}
	if err := FromMillis(10, 5, 0.2).Validate(); err != nil {
		t.Errorf("valid config rejected: %v", err)
	}
}

func TestPartitionReachability(t *testing.T) {
	n, err := New(Config{}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := n.SetPartition([][]raft.NodeID{{"a", "b"}, {"c"}}); err != nil {
		t.Fatal(err)
	}
	if !n.Reachable("a", "b") || n.Reachable("a", "c") || n.Reachable("c", "a") {
		t.Error("explicit groups not respected")
	}
	if n.Reachable("a", "d") || !n.Reachable("d", "e") {
		t.Error("unlisted nodes should form one extra group")
	}
	n.Heal()
	if !n.Reachable("a", "c") || n.Partitioned() {
		t.Error("heal did not restore connectivity")
	}
	if n.SetPartition([][]raft.NodeID{{"a"}, {"a"}}) == nil {
		t.Error("node in two groups accepted")
	}
}

func TestRouteIsDeterministicPerSeed(t *testing.T) {
	cfg := FromMillis(10, 5, 0.3)
	a, _ := New(cfg, 42)
	b, _ := New(cfg, 42)
	for i := 0; i < 200; i++ {
		da, db := a.Route("x", "y"), b.Route("x", "y")
		if da != db {
			t.Fatalf("decision %d differs: %+v vs %+v", i, da, db)
		}
		if da.Deliver && (da.Delay < cfg.BaseDelay || da.Delay > cfg.BaseDelay+cfg.Jitter) {
			t.Fatalf("delay %v outside [base, base+jitter]", da.Delay)
		}
	}
}

func TestTotalLossAndStats(t *testing.T) {
	n, _ := New(Config{LossProbability: 1}, 1)
	for i := 0; i < 10; i++ {
		if n.Route("a", "b").Deliver {
			t.Fatal("message delivered with loss probability 1")
		}
	}
	st := n.Stats()
	if st.Sent != 10 || st.DroppedLoss != 10 {
		t.Fatalf("stats = %+v", st)
	}
	if _, ok := st.DeliveryRate(); !ok {
		t.Error("delivery rate should be defined once messages were sent")
	}
	if _, ok := (Stats{}).DeliveryRate(); ok {
		t.Error("delivery rate must be undefined when nothing was sent")
	}
}

func TestPartitionDropsMessages(t *testing.T) {
	n, _ := New(Config{}, 1)
	_ = n.SetPartition([][]raft.NodeID{{"a"}, {"b"}})
	d := n.Route("a", "b")
	if d.Deliver || d.Reason != "partition" || n.Stats().DroppedPartition != 1 {
		t.Fatalf("decision=%+v stats=%+v", d, n.Stats())
	}
}
