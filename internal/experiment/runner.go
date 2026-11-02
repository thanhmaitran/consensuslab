package experiment

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/thanhmaitran/consensuslab/internal/observability"
	"github.com/thanhmaitran/consensuslab/internal/raft"
	"github.com/thanhmaitran/consensuslab/internal/sim"
	"github.com/thanhmaitran/consensuslab/internal/verify"
)

// RunRecord is one line of runs.jsonl. Pointer fields are null when the
// quantity could not be measured in that run; they are never filled with 0.
type RunRecord struct {
	ExperimentID string `json:"experiment_id"`
	Variant      string `json:"variant"`
	Seed         int64  `json:"seed"`
	NodeCount    int    `json:"node_count"`
	Fault        string `json:"fault"`
	Status       string `json:"status"` // "ok" or "error"
	Error        string `json:"error,omitempty"`

	ElectionLatencyMs           *float64 `json:"election_latency_ms"`
	ReelectionLatencyMs         *float64 `json:"reelection_latency_ms"`
	CommitLatencyMsP50          *float64 `json:"commit_latency_ms_p50"`
	CommitLatencyMsP95          *float64 `json:"commit_latency_ms_p95"`
	CommitLatencyMsP99          *float64 `json:"commit_latency_ms_p99"`
	CommandsSubmitted           int      `json:"commands_submitted"`
	CommandsCommitted           int      `json:"commands_committed"`
	ThroughputCommandsPerSecond *float64 `json:"throughput_commands_per_second"`
	RecoveryTimeMs              *float64 `json:"recovery_time_ms"`
	ElectionsStarted            int      `json:"elections_started"`
	FailedElections             int      `json:"failed_elections"`
	MessagesSent                uint64   `json:"messages_sent"`
	MessagesDelivered           uint64   `json:"messages_delivered"`
	MessageDeliveryRate         *float64 `json:"message_delivery_rate"`
	SafetyViolations            int      `json:"safety_violations"`
	TraceHash                   string   `json:"trace_hash"`
	CodeVersion                 string   `json:"code_version"`
}

// Result is the full outcome of one run.
type Result struct {
	Record RunRecord
	Report verify.Report
	Events []observability.Event
	Inputs []sim.Input
}

func f64(v float64) *float64 { return &v }

func toMs(d time.Duration) float64 { return float64(d) / float64(time.Millisecond) }

func faultLabel(s *Scenario) string {
	if len(s.Faults) == 0 {
		return "none"
	}
	names := make([]string, len(s.Faults))
	for i, f := range s.Faults {
		names[i] = f.Type
	}
	return strings.Join(names, "+")
}

// RunOne executes one (node count, variant, seed) combination.
func RunOne(s *Scenario, spec RunSpec, keepEvents bool, codeVersion string) (*Result, error) {
	cfg := sim.DefaultConfig()
	cfg.ScenarioID = s.ID
	cfg.Seed = spec.Seed
	cfg.NodeCount = spec.NodeCount
	cfg.Network = spec.Variant.Network
	cfg.ElectionTimeoutMin = ms(s.ElectionTimeoutMs[0])
	cfg.ElectionTimeoutMax = ms(s.ElectionTimeoutMs[1])
	cfg.HeartbeatInterval = ms(s.HeartbeatMs)
	cfg.MaxVirtualTime = ms(s.DurationMs)
	cfg.MaxEvents = 200000

	eng, err := sim.NewEngine(cfg)
	if err != nil {
		return nil, err
	}

	for i := 0; i < s.Workload.Commands; i++ {
		at := ms(s.Workload.StartMs + float64(i)*s.Workload.IntervalMs)
		if _, err := eng.ScheduleCommand(at, "", fmt.Sprintf("SET key-%d value-%d", i, i)); err != nil {
			return nil, err
		}
	}

	var firstFault, removal, lastCrash time.Duration
	haveFault, haveRemoval, haveCrash := false, false, false
	for _, f := range s.Faults {
		at := ms(f.AtMs)
		if !haveFault || at < firstFault {
			firstFault, haveFault = at, true
		}
		restartAfter := ms(f.RestartAfterMs)
		switch f.Type {
		case FaultCrash:
			err = eng.ScheduleCrash(at, raft.NodeID(f.NodeID), restartAfter)
		case FaultCrashLeader:
			err = eng.ScheduleCrash(at, "", restartAfter)
		case FaultRestart:
			err = eng.ScheduleRestart(at, raft.NodeID(f.NodeID))
		case FaultPartition:
			err = eng.SchedulePartitionSizes(at, f.GroupSizes, f.IsolateLeader)
		case FaultHeal:
			eng.ScheduleHeal(at)
		}
		if err != nil {
			return nil, err
		}
		markRemoval := func(t time.Duration) {
			if !haveRemoval || t > removal {
				removal, haveRemoval = t, true
			}
		}
		switch {
		case (f.Type == FaultCrash || f.Type == FaultCrashLeader) && f.RestartAfterMs > 0:
			markRemoval(at + restartAfter)
		case f.Type == FaultRestart || f.Type == FaultHeal:
			markRemoval(at)
		}
		if f.Type == FaultCrash || f.Type == FaultCrashLeader {
			if !haveCrash || at > lastCrash {
				lastCrash, haveCrash = at, true
			}
		}
	}
	if !haveRemoval && haveCrash {
		removal, haveRemoval = lastCrash, true // permanent crash: measure from the crash itself
	}

	var recovery *time.Duration
	eng.OnStep = func() {
		if recovery == nil && haveRemoval && eng.Now >= removal && eng.ClusterStable() {
			d := eng.Now - removal
			recovery = &d
		}
	}

	eng.RunUntil(cfg.MaxVirtualTime)
	report := eng.Finish()
	m := eng.Metrics()
	netStats := eng.Network.Stats()

	rec := RunRecord{
		ExperimentID:      s.ID,
		Variant:           spec.Variant.Name,
		Seed:              spec.Seed,
		NodeCount:         spec.NodeCount,
		Fault:             faultLabel(s),
		Status:            "ok",
		CommandsSubmitted: m.CommandsSubmitted,
		CommandsCommitted: m.CommandsCommitted,
		ElectionsStarted:  m.ElectionsStarted,
		MessagesSent:      netStats.Sent,
		MessagesDelivered: netStats.Delivered,
		SafetyViolations:  len(report.Violations),
		TraceHash:         eng.Recorder.TraceHash(),
		CodeVersion:       codeVersion,
	}

	if len(m.Elections) > 0 {
		rec.ElectionLatencyMs = f64(toMs(m.Elections[0].At))
	}
	if haveFault {
		for _, el := range m.Elections {
			if el.At > firstFault {
				rec.ReelectionLatencyMs = f64(toMs(el.At - firstFault))
				break
			}
		}
	}
	terms := make(map[uint64]bool)
	for _, el := range m.Elections {
		terms[el.Term] = true
	}
	if failed := m.ElectionsStarted - len(terms); failed > 0 {
		rec.FailedElections = failed
	}
	if len(m.CommitLatencies) > 0 {
		lat := make([]float64, len(m.CommitLatencies))
		for i, d := range m.CommitLatencies {
			lat[i] = toMs(d)
		}
		if sum, ok := observability.Summarize(lat); ok {
			rec.CommitLatencyMsP50 = f64(sum.Median)
			rec.CommitLatencyMsP95 = sum.P95
			rec.CommitLatencyMsP99 = sum.P99
		}
	}
	if m.CommandsCommitted >= 2 && m.LastCommit > m.FirstAccept {
		rec.ThroughputCommandsPerSecond = f64(float64(m.CommandsCommitted) / (m.LastCommit - m.FirstAccept).Seconds())
	}
	if recovery != nil {
		rec.RecoveryTimeMs = f64(toMs(*recovery))
	}
	if rate, ok := netStats.DeliveryRate(); ok {
		rec.MessageDeliveryRate = f64(rate)
	}

	res := &Result{Record: rec, Report: report, Inputs: eng.Inputs()}
	if keepEvents {
		res.Events = eng.Recorder.All()
	}
	return res, nil
}

// Output is the outcome of running a whole scenario.
type Output struct {
	Scenario   *Scenario
	Records    []RunRecord
	Aggregates []Aggregate
	Failures   []*Result // runs with safety violations
}

// Options controls RunAll.
type Options struct {
	OutDir      string
	Workers     int
	CodeVersion string
	// Progress is called (serialised) after every finished run.
	Progress func(done, total int)
}

// RunAll executes every run of the scenario. Runs are independent, so
// results do not depend on the number of workers.
func RunAll(s *Scenario, opts Options) (*Output, error) {
	if err := s.Validate(); err != nil {
		return nil, err
	}
	plan := s.Plan()
	records := make([]RunRecord, len(plan))
	failed := make([]*Result, len(plan))

	workers := opts.Workers
	if workers < 1 {
		workers = 1
	}
	var mu sync.Mutex
	done := 0
	jobs := make(chan int)
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				spec := plan[i]
				res, err := RunOne(s, spec, false, opts.CodeVersion)
				if err != nil {
					records[i] = RunRecord{
						ExperimentID: s.ID, Variant: spec.Variant.Name, Seed: spec.Seed,
						NodeCount: spec.NodeCount, Fault: faultLabel(s), Status: "error",
						Error: err.Error(), CodeVersion: opts.CodeVersion,
					}
				} else {
					records[i] = res.Record
					if res.Report.Status != "passed" {
						failed[i] = res
					}
				}
				mu.Lock()
				done++
				if opts.Progress != nil {
					opts.Progress(done, len(plan))
				}
				mu.Unlock()
			}
		}()
	}
	for i := range plan {
		jobs <- i
	}
	close(jobs)
	wg.Wait()

	out := &Output{Scenario: s, Records: records, Aggregates: ComputeAggregates(records)}
	for _, f := range failed {
		if f != nil {
			out.Failures = append(out.Failures, f)
		}
	}
	if opts.OutDir != "" {
		if err := WriteOutputs(opts.OutDir, out); err != nil {
			return out, err
		}
	}
	return out, nil
}
