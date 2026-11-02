package observability

import (
	"math"
	"testing"
)

func TestSummarize(t *testing.T) {
	s, ok := Summarize([]float64{1, 2, 3, 4, 5})
	if !ok {
		t.Fatal("no summary")
	}
	if s.N != 5 || s.Mean != 3 || s.Median != 3 || s.Min != 1 || s.Max != 5 {
		t.Fatalf("summary = %+v", s)
	}
	if math.Abs(s.StdDev-math.Sqrt(2.5)) > 1e-9 {
		t.Errorf("std dev = %v", s.StdDev)
	}
	if s.P95 != nil || s.P99 != nil {
		t.Error("tail percentiles must be nil for tiny samples")
	}
	if s.CI95Half == nil {
		t.Error("expected a confidence interval for n >= 2")
	}
	if _, ok := Summarize(nil); ok {
		t.Error("empty sample should not summarise")
	}
}

func TestTailPercentilesNeedEnoughSamples(t *testing.T) {
	vals := make([]float64, 100)
	for i := range vals {
		vals[i] = float64(i + 1)
	}
	s, _ := Summarize(vals)
	if s.P95 == nil || s.P99 == nil {
		t.Fatal("expected P95 and P99 for 100 samples")
	}
	if math.Abs(*s.P95-95.05) > 1e-9 {
		t.Errorf("P95 = %v", *s.P95)
	}
}

func TestPercentileEdgeCases(t *testing.T) {
	if !math.IsNaN(Percentile(nil, 50)) {
		t.Error("empty slice should give NaN")
	}
	if Percentile([]float64{7}, 99) != 7 {
		t.Error("single value should be returned as is")
	}
}

func TestRecorderHashIsDeterministic(t *testing.T) {
	run := func() string {
		r := NewRecorder(10, "s", 1)
		for i := 0; i < 5; i++ {
			r.Record(Event{Type: "X", Term: uint64(i), Metadata: map[string]any{"b": 1, "a": 2}})
		}
		return r.TraceHash()
	}
	if run() != run() {
		t.Fatal("same events produced different hashes")
	}
	r := NewRecorder(10, "s", 2)
	r.Record(Event{Type: "X"})
	if r.TraceHash() == run() {
		t.Fatal("different histories share a hash")
	}
}

func TestRecorderEvictsOldEventsButKeepsIDs(t *testing.T) {
	r := NewRecorder(20, "s", 1)
	for i := 0; i < 50; i++ {
		r.Record(Event{Type: "X"})
	}
	if r.Len() > 20 || r.Dropped() == 0 {
		t.Fatalf("len=%d dropped=%d", r.Len(), r.Dropped())
	}
	all := r.All()
	for i := 1; i < len(all); i++ {
		if all[i].ID != all[i-1].ID+1 {
			t.Fatal("event IDs are not contiguous")
		}
	}
	if all[len(all)-1].ID != 50 {
		t.Fatalf("last ID = %d, want 50", all[len(all)-1].ID)
	}
	tail := r.Since(48, 0)
	if len(tail) != 2 || tail[0].ID != 49 {
		t.Fatalf("Since(48) = %+v", tail)
	}
}
