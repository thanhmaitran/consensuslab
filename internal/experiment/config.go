// Package experiment turns JSON scenario files into reproducible seeded runs,
// collects metrics and writes JSONL/CSV/Markdown results.
package experiment

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/thanhmaitran/consensuslab/internal/network"
	"github.com/thanhmaitran/consensuslab/internal/sim"
)

// Limits that keep experiments (and the public API) bounded.
const (
	MaxSeeds      = 1000
	MaxDurationMs = 3600000
	MaxCommands   = 10000
)

type NetworkJSON struct {
	BaseDelayMs     float64 `json:"base_delay_ms"`
	JitterMs        float64 `json:"jitter_ms"`
	LossProbability float64 `json:"message_loss_probability"`
}

func (n NetworkJSON) Config() network.Config {
	return network.FromMillis(n.BaseDelayMs, n.JitterMs, n.LossProbability)
}

// Fault types understood by the runner.
const (
	FaultCrash       = "crash"
	FaultCrashLeader = "crash_leader"
	FaultRestart     = "restart"
	FaultPartition   = "partition"
	FaultHeal        = "heal"
)

type FaultJSON struct {
	AtMs           float64 `json:"at_ms"`
	Type           string  `json:"type"`
	NodeID         string  `json:"node_id,omitempty"`
	RestartAfterMs float64 `json:"restart_after_ms,omitempty"`
	GroupSizes     []int   `json:"group_sizes,omitempty"`
	IsolateLeader  bool    `json:"isolate_leader,omitempty"`
}

type WorkloadJSON struct {
	Commands   int     `json:"commands"`
	StartMs    float64 `json:"start_ms"`
	IntervalMs float64 `json:"interval_ms"`
}

type VariantJSON struct {
	Name    string      `json:"name"`
	Network NetworkJSON `json:"network"`
}

// Scenario is the on-disk experiment definition.
type Scenario struct {
	ID                string        `json:"id"`
	Description       string        `json:"description"`
	NodeCounts        []int         `json:"node_counts"`
	Seeds             int           `json:"seeds"`
	SeedBase          int64         `json:"seed_base"`
	DurationMs        float64       `json:"duration_ms"`
	ElectionTimeoutMs [2]float64    `json:"election_timeout_ms"`
	HeartbeatMs       float64       `json:"heartbeat_ms"`
	Network           NetworkJSON   `json:"network"`
	Variants          []VariantJSON `json:"variants,omitempty"`
	Faults            []FaultJSON   `json:"faults,omitempty"`
	Workload          WorkloadJSON  `json:"workload"`
}

// Variant is a named network setting resolved from the scenario.
type Variant struct {
	Name    string
	Network network.Config
}

// RunSpec identifies a single run inside a scenario.
type RunSpec struct {
	NodeCount int
	Variant   Variant
	Seed      int64
}

// Load reads and validates a scenario file; unknown fields are rejected.
func Load(path string) (*Scenario, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(data)
}

func Parse(data []byte) (*Scenario, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var s Scenario
	if err := dec.Decode(&s); err != nil {
		return nil, fmt.Errorf("experiment: invalid scenario JSON: %w", err)
	}
	if err := s.Validate(); err != nil {
		return nil, err
	}
	return &s, nil
}

// Validate applies defaults and rejects inconsistent scenarios.
func (s *Scenario) Validate() error {
	if s.ID == "" {
		return errors.New("experiment: scenario id is required")
	}
	if len(s.NodeCounts) == 0 {
		return errors.New("experiment: node_counts must not be empty")
	}
	for _, n := range s.NodeCounts {
		if n < 1 || n > sim.MaxNodes {
			return fmt.Errorf("experiment: node count %d is outside [1, %d]", n, sim.MaxNodes)
		}
	}
	if s.Seeds < 1 || s.Seeds > MaxSeeds {
		return fmt.Errorf("experiment: seeds must be in [1, %d]", MaxSeeds)
	}
	if s.SeedBase == 0 {
		s.SeedBase = 1
	}
	if s.DurationMs <= 0 || s.DurationMs > MaxDurationMs {
		return fmt.Errorf("experiment: duration_ms must be in (0, %d]", MaxDurationMs)
	}
	if s.ElectionTimeoutMs == [2]float64{} {
		s.ElectionTimeoutMs = [2]float64{150, 300}
	}
	if s.HeartbeatMs == 0 {
		s.HeartbeatMs = 50
	}
	if s.HeartbeatMs <= 0 || s.ElectionTimeoutMs[0] <= s.HeartbeatMs || s.ElectionTimeoutMs[1] < s.ElectionTimeoutMs[0] {
		return errors.New("experiment: need heartbeat_ms < election_timeout_ms[0] <= election_timeout_ms[1]")
	}
	if err := s.Network.Config().Validate(); err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, v := range s.Variants {
		if v.Name == "" || seen[v.Name] {
			return errors.New("experiment: variants need unique, non-empty names")
		}
		seen[v.Name] = true
		if err := v.Network.Config().Validate(); err != nil {
			return fmt.Errorf("experiment: variant %q: %w", v.Name, err)
		}
	}
	if s.Workload.Commands < 0 || s.Workload.Commands > MaxCommands {
		return fmt.Errorf("experiment: workload.commands must be in [0, %d]", MaxCommands)
	}
	if s.Workload.Commands > 1 && s.Workload.IntervalMs <= 0 {
		return errors.New("experiment: workload.interval_ms must be positive")
	}
	if s.Workload.StartMs < 0 {
		return errors.New("experiment: workload.start_ms must not be negative")
	}
	for i, f := range s.Faults {
		if err := s.validateFault(f); err != nil {
			return fmt.Errorf("experiment: fault %d: %w", i, err)
		}
	}
	return nil
}

func (s *Scenario) validateFault(f FaultJSON) error {
	if f.AtMs < 0 || f.AtMs > s.DurationMs {
		return errors.New("at_ms must lie within the scenario duration")
	}
	if f.RestartAfterMs < 0 {
		return errors.New("restart_after_ms must not be negative")
	}
	switch f.Type {
	case FaultCrash, FaultRestart:
		if f.NodeID == "" {
			return fmt.Errorf("%s requires node_id", f.Type)
		}
		for _, n := range s.NodeCounts {
			ok := false
			for i := 1; i <= n; i++ {
				if string(sim.NodeName(i)) == f.NodeID {
					ok = true
				}
			}
			if !ok {
				return fmt.Errorf("node %q does not exist in a %d-node cluster", f.NodeID, n)
			}
		}
	case FaultCrashLeader, FaultHeal:
	case FaultPartition:
		if len(f.GroupSizes) < 2 {
			return errors.New("partition requires at least two group_sizes")
		}
		sum := 0
		for _, g := range f.GroupSizes {
			if g < 1 {
				return errors.New("group_sizes must be positive")
			}
			sum += g
		}
		for _, n := range s.NodeCounts {
			if sum != n {
				return fmt.Errorf("group_sizes sum to %d but node_counts contains %d", sum, n)
			}
		}
	default:
		return fmt.Errorf("unknown fault type %q", f.Type)
	}
	return nil
}

// ResolveVariants returns the scenario's network variants, or a single
// "default" variant built from the top-level network settings.
func (s *Scenario) ResolveVariants() []Variant {
	if len(s.Variants) == 0 {
		return []Variant{{Name: "default", Network: s.Network.Config()}}
	}
	out := make([]Variant, len(s.Variants))
	for i, v := range s.Variants {
		out[i] = Variant{Name: v.Name, Network: v.Network.Config()}
	}
	return out
}

// Plan lists every run in a deterministic order.
func (s *Scenario) Plan() []RunSpec {
	base := s.SeedBase
	if base == 0 {
		base = 1
	}
	var plan []RunSpec
	for _, n := range s.NodeCounts {
		for _, v := range s.ResolveVariants() {
			for k := 0; k < s.Seeds; k++ {
				plan = append(plan, RunSpec{NodeCount: n, Variant: v, Seed: base + int64(k)})
			}
		}
	}
	return plan
}

func ms(v float64) time.Duration { return time.Duration(v * float64(time.Millisecond)) }
