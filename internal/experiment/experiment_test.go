package experiment

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

const smokePath = "../../configs/smoke.json"

func TestAllShippedConfigsAreValid(t *testing.T) {
	files, err := filepath.Glob("../../configs/*.json")
	if err != nil || len(files) == 0 {
		t.Fatalf("no configs found: %v", err)
	}
	for _, f := range files {
		if _, err := Load(f); err != nil {
			t.Errorf("%s: %v", f, err)
		}
	}
}

func TestValidationRejectsBadScenarios(t *testing.T) {
	cases := map[string]string{
		"unknown field":      `{"id":"X","node_counts":[3],"seeds":1,"duration_ms":1000,"bogus":1}`,
		"no id":              `{"node_counts":[3],"seeds":1,"duration_ms":1000}`,
		"bad node count":     `{"id":"X","node_counts":[9],"seeds":1,"duration_ms":1000}`,
		"bad loss":           `{"id":"X","node_counts":[3],"seeds":1,"duration_ms":1000,"network":{"message_loss_probability":2}}`,
		"partition mismatch": `{"id":"X","node_counts":[5],"seeds":1,"duration_ms":1000,"faults":[{"at_ms":1,"type":"partition","group_sizes":[3,3]}]}`,
		"unknown fault":      `{"id":"X","node_counts":[3],"seeds":1,"duration_ms":1000,"faults":[{"at_ms":1,"type":"explode"}]}`,
		"fault after end":    `{"id":"X","node_counts":[3],"seeds":1,"duration_ms":1000,"faults":[{"at_ms":5000,"type":"heal"}]}`,
		"crash unknown node": `{"id":"X","node_counts":[3],"seeds":1,"duration_ms":1000,"faults":[{"at_ms":1,"type":"crash","node_id":"node-5"}]}`,
	}
	for name, body := range cases {
		if _, err := Parse([]byte(body)); err == nil {
			t.Errorf("%s: scenario accepted", name)
		}
	}
}

func TestRunOneIsSafeAndReproducible(t *testing.T) {
	s, err := Load(smokePath)
	if err != nil {
		t.Fatal(err)
	}
	spec := s.Plan()[0]
	a, err := RunOne(s, spec, false, "test")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := RunOne(s, spec, false, "test")
	if a.Report.Status != "passed" || a.Record.SafetyViolations != 0 {
		t.Fatalf("violations: %+v", a.Report.Violations)
	}
	if a.Record.TraceHash != b.Record.TraceHash {
		t.Fatal("same seed produced different trace hashes")
	}
	if a.Record.ElectionLatencyMs == nil {
		t.Fatal("no leader was elected in the smoke scenario")
	}
}

func TestRunAllIsIndependentOfWorkerCount(t *testing.T) {
	s, _ := Load(smokePath)
	one, err := RunAll(s, Options{Workers: 1})
	if err != nil {
		t.Fatal(err)
	}
	many, err := RunAll(s, Options{Workers: 3})
	if err != nil {
		t.Fatal(err)
	}
	x, _ := json.Marshal(one.Records)
	y, _ := json.Marshal(many.Records)
	if string(x) != string(y) {
		t.Fatal("results depend on the number of workers")
	}
}

func TestWriteOutputs(t *testing.T) {
	s, _ := Load(smokePath)
	dir := t.TempDir()
	if _, err := RunAll(s, Options{OutDir: dir, CodeVersion: "test"}); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"scenario.json", "runs.jsonl", "summary.csv", "report.md"} {
		if st, err := os.Stat(filepath.Join(dir, f)); err != nil || st.Size() == 0 {
			t.Errorf("%s missing or empty: %v", f, err)
		}
	}
}

func TestAggregatesIgnoreUnmeasuredValues(t *testing.T) {
	v := 10.0
	recs := []RunRecord{
		{ExperimentID: "X", NodeCount: 3, Variant: "v", Status: "ok", ElectionLatencyMs: &v},
		{ExperimentID: "X", NodeCount: 3, Variant: "v", Status: "ok"},
		{ExperimentID: "X", NodeCount: 3, Variant: "v", Status: "error"},
	}
	for _, a := range ComputeAggregates(recs) {
		if a.Metric != "election_latency_ms" {
			continue
		}
		if a.Runs != 3 || a.Measured != 1 || a.Summary == nil || a.Summary.Mean != 10 {
			t.Fatalf("aggregate = %+v", a)
		}
		return
	}
	t.Fatal("election_latency_ms aggregate missing")
}
