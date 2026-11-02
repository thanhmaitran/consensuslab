package experiment

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/thanhmaitran/consensuslab/internal/observability"
)

// Aggregate summarises one metric over all seeds of one (node count, variant).
type Aggregate struct {
	ExperimentID string                 `json:"experiment_id"`
	NodeCount    int                    `json:"node_count"`
	Variant      string                 `json:"variant"`
	Metric       string                 `json:"metric"`
	Runs         int                    `json:"runs"`
	Measured     int                    `json:"measured"`
	Summary      *observability.Summary `json:"summary"`
}

type metricDef struct {
	name string
	get  func(RunRecord) *float64
}

var metricDefs = []metricDef{
	{"election_latency_ms", func(r RunRecord) *float64 { return r.ElectionLatencyMs }},
	{"reelection_latency_ms", func(r RunRecord) *float64 { return r.ReelectionLatencyMs }},
	{"commit_latency_ms_p50", func(r RunRecord) *float64 { return r.CommitLatencyMsP50 }},
	{"throughput_commands_per_second", func(r RunRecord) *float64 { return r.ThroughputCommandsPerSecond }},
	{"recovery_time_ms", func(r RunRecord) *float64 { return r.RecoveryTimeMs }},
	{"message_delivery_rate", func(r RunRecord) *float64 { return r.MessageDeliveryRate }},
	{"failed_elections", func(r RunRecord) *float64 { return f64(float64(r.FailedElections)) }},
}

// ComputeAggregates groups ok runs by (node count, variant) in first-seen order.
func ComputeAggregates(records []RunRecord) []Aggregate {
	type key struct {
		n       int
		variant string
	}
	groups := map[key][]RunRecord{}
	var order []key
	for _, r := range records {
		k := key{r.NodeCount, r.Variant}
		if _, ok := groups[k]; !ok {
			order = append(order, k)
		}
		groups[k] = append(groups[k], r)
	}
	var out []Aggregate
	for _, k := range order {
		runs := groups[k]
		for _, md := range metricDefs {
			var vals []float64
			for _, r := range runs {
				if r.Status != "ok" {
					continue
				}
				if p := md.get(r); p != nil {
					vals = append(vals, *p)
				}
			}
			agg := Aggregate{
				ExperimentID: runs[0].ExperimentID, NodeCount: k.n, Variant: k.variant,
				Metric: md.name, Runs: len(runs), Measured: len(vals),
			}
			if sum, ok := observability.Summarize(vals); ok {
				s := sum
				agg.Summary = &s
			}
			out = append(out, agg)
		}
	}
	return out
}

func optF(p *float64) string {
	if p == nil {
		return ""
	}
	return strconv.FormatFloat(*p, 'g', 8, 64)
}

func fv(v float64) string { return strconv.FormatFloat(v, 'g', 8, 64) }

// WriteOutputs writes scenario.json, runs.jsonl, summary.csv, report.md and
// one file per run that violated an invariant (failures/).
func WriteOutputs(dir string, out *Output) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	sc, err := json.MarshalIndent(out.Scenario, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "scenario.json"), append(sc, '\n'), 0o644); err != nil {
		return err
	}

	rf, err := os.Create(filepath.Join(dir, "runs.jsonl"))
	if err != nil {
		return err
	}
	enc := json.NewEncoder(rf)
	for _, r := range out.Records {
		if err := enc.Encode(r); err != nil {
			rf.Close()
			return err
		}
	}
	if err := rf.Close(); err != nil {
		return err
	}

	cf, err := os.Create(filepath.Join(dir, "summary.csv"))
	if err != nil {
		return err
	}
	w := csv.NewWriter(cf)
	_ = w.Write([]string{"experiment_id", "node_count", "variant", "metric", "runs", "measured",
		"mean", "std_dev", "median", "min", "max", "p95", "p99", "ci95_half"})
	for _, a := range out.Aggregates {
		row := []string{a.ExperimentID, strconv.Itoa(a.NodeCount), a.Variant, a.Metric,
			strconv.Itoa(a.Runs), strconv.Itoa(a.Measured)}
		if a.Summary == nil {
			row = append(row, "", "", "", "", "", "", "", "")
		} else {
			s := a.Summary
			row = append(row, fv(s.Mean), fv(s.StdDev), fv(s.Median), fv(s.Min), fv(s.Max),
				optF(s.P95), optF(s.P99), optF(s.CI95Half))
		}
		_ = w.Write(row)
	}
	w.Flush()
	if err := w.Error(); err != nil {
		cf.Close()
		return err
	}
	if err := cf.Close(); err != nil {
		return err
	}

	if err := os.WriteFile(filepath.Join(dir, "report.md"), []byte(Markdown(out)), 0o644); err != nil {
		return err
	}

	if len(out.Failures) > 0 {
		fdir := filepath.Join(dir, "failures")
		if err := os.MkdirAll(fdir, 0o755); err != nil {
			return err
		}
		for _, f := range out.Failures {
			name := fmt.Sprintf("%s_n%d_%s_seed%d.json", f.Record.ExperimentID, f.Record.NodeCount, f.Record.Variant, f.Record.Seed)
			payload := map[string]any{
				"record":   f.Record,
				"report":   f.Report,
				"inputs":   f.Inputs,
				"scenario": out.Scenario,
				"replay":   "consensuslab replay -config scenario.json -nodes N -variant NAME -seed SEED -expect TRACE_HASH",
			}
			data, err := json.MarshalIndent(payload, "", "  ")
			if err != nil {
				return err
			}
			if err := os.WriteFile(filepath.Join(fdir, name), data, 0o644); err != nil {
				return err
			}
		}
	}
	return nil
}

// Markdown renders a human-readable report from measured data only.
func Markdown(out *Output) string {
	s := out.Scenario
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", s.ID)
	if s.Description != "" {
		fmt.Fprintf(&b, "%s\n\n", s.Description)
	}
	b.WriteString("All times are **virtual milliseconds** from the deterministic simulator. ")
	b.WriteString("They are not wall-clock measurements and must not be compared with process-based results.\n\n")
	fmt.Fprintf(&b, "- Seeds per configuration: %d (base seed %d)\n", s.Seeds, s.SeedBase)
	fmt.Fprintf(&b, "- Duration per run: %s ms\n", fv(s.DurationMs))
	fmt.Fprintf(&b, "- Election timeout: %s-%s ms, heartbeat: %s ms\n", fv(s.ElectionTimeoutMs[0]), fv(s.ElectionTimeoutMs[1]), fv(s.HeartbeatMs))
	fmt.Fprintf(&b, "- Fault schedule: %s\n", faultLabel(s))

	ok, errs, viol := 0, 0, 0
	for _, r := range out.Records {
		if r.Status == "ok" {
			ok++
		} else {
			errs++
		}
		viol += r.SafetyViolations
	}
	fmt.Fprintf(&b, "- Runs: %d ok, %d errors, %d safety violations in total\n\n", ok, errs, viol)

	b.WriteString("## Results (median / mean ± std dev, measured runs of total)\n\n")
	b.WriteString("| Nodes | Variant | Metric | Measured | Median | Mean ± SD |\n|---|---|---|---|---|---|\n")
	for _, a := range out.Aggregates {
		if a.Summary == nil {
			fmt.Fprintf(&b, "| %d | %s | %s | 0/%d | not measured | not measured |\n", a.NodeCount, a.Variant, a.Metric, a.Runs)
			continue
		}
		fmt.Fprintf(&b, "| %d | %s | %s | %d/%d | %.2f | %.2f ± %.2f |\n",
			a.NodeCount, a.Variant, a.Metric, a.Measured, a.Runs, a.Summary.Median, a.Summary.Mean, a.Summary.StdDev)
	}
	b.WriteString("\nPercentiles (P95/P99) and confidence intervals are in `summary.csv`; ")
	b.WriteString("P95 needs at least 20 samples and P99 at least 100.\n")
	return b.String()
}
