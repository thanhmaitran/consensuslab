// Package verify checks Raft safety invariants on every observed state change.
// It is independent of the Raft implementation: it only reads snapshots.
package verify

import (
	"fmt"
	"time"

	"github.com/thanhmaitran/consensuslab/internal/raft"
)

// Invariant names reported in violations.
const (
	ElectionSafety     = "election_safety"
	LogMatching        = "log_matching"
	LeaderCompleteness = "leader_completeness"
	StateMachineSafety = "state_machine_safety"
	IndexBounds        = "index_bounds"
)

// InvariantsChecked is the number of distinct invariants this verifier checks.
const InvariantsChecked = 5

const maxViolations = 100

// Violation describes one detected invariant failure.
type Violation struct {
	Invariant     string  `json:"invariant"`
	NodeID        string  `json:"node_id,omitempty"`
	Term          uint64  `json:"term,omitempty"`
	Index         uint64  `json:"index,omitempty"`
	VirtualTimeMs float64 `json:"virtual_time_ms"`
	Detail        string  `json:"detail"`
}

// Report is the structured verifier output.
type Report struct {
	ScenarioID        string      `json:"scenario_id"`
	Seed              int64       `json:"seed"`
	InvariantsChecked int         `json:"invariants_checked"`
	Observations      uint64      `json:"observations"`
	Violations        []Violation `json:"violations"`
	Status            string      `json:"status"`
}

// Verifier accumulates evidence across the whole run, so a violation that
// later disappears from the node state is still reported.
type Verifier struct {
	scenario string
	seed     int64

	leaders      map[uint64]raft.NodeID // term -> first leader observed
	committed    map[uint64]raft.LogEntry
	checked      map[raft.NodeID]uint64 // highest commit index already verified per node
	violations   []Violation
	observations uint64

	// OnViolation, if set, is called for each new violation.
	OnViolation func(Violation)
}

func New(scenario string, seed int64) *Verifier {
	return &Verifier{
		scenario:  scenario,
		seed:      seed,
		leaders:   make(map[uint64]raft.NodeID),
		committed: make(map[uint64]raft.LogEntry),
		checked:   make(map[raft.NodeID]uint64),
	}
}

func (v *Verifier) add(now time.Duration, inv string, node raft.NodeID, term, index uint64, format string, args ...any) {
	viol := Violation{
		Invariant:     inv,
		NodeID:        string(node),
		Term:          term,
		Index:         index,
		VirtualTimeMs: float64(now) / float64(time.Millisecond),
		Detail:        fmt.Sprintf(format, args...),
	}
	if len(v.violations) < maxViolations {
		v.violations = append(v.violations, viol)
	}
	if v.OnViolation != nil {
		v.OnViolation(viol)
	}
}

// ViolationCount returns the number of retained violations.
func (v *Verifier) ViolationCount() int { return len(v.violations) }

// ObserveNode checks the invariants that can be decided from one node's
// state. The engine calls it after every event that touched the node.
func (v *Verifier) ObserveNode(now time.Duration, n raft.Node) {
	v.observations++
	snap := n.Snapshot()
	st := snap.Status
	lastIdx := uint64(len(snap.Log) - 1)

	// Index bounds and log structure.
	if st.LastApplied > st.CommitIndex || st.CommitIndex > lastIdx {
		v.add(now, IndexBounds, st.ID, st.CurrentTerm, st.CommitIndex,
			"lastApplied=%d commitIndex=%d lastLogIndex=%d violates 0<=lastApplied<=commitIndex<=lastLogIndex",
			st.LastApplied, st.CommitIndex, lastIdx)
	}
	if lastIdx >= 1 && snap.Log[lastIdx].Index != lastIdx {
		v.add(now, IndexBounds, st.ID, st.CurrentTerm, lastIdx,
			"last entry carries index %d but sits at position %d", snap.Log[lastIdx].Index, lastIdx)
	}
	if lastIdx >= 2 && snap.Log[lastIdx].Term < snap.Log[lastIdx-1].Term {
		v.add(now, IndexBounds, st.ID, st.CurrentTerm, lastIdx, "log terms decrease at index %d", lastIdx)
	}

	// Election safety and leader completeness (checked once per term).
	if st.Role == raft.Leader.String() {
		if prev, ok := v.leaders[st.CurrentTerm]; ok {
			if prev != st.ID {
				v.add(now, ElectionSafety, st.ID, st.CurrentTerm, 0,
					"%s and %s are both leader in term %d", prev, st.ID, st.CurrentTerm)
			}
		} else {
			v.leaders[st.CurrentTerm] = st.ID
			v.checkLeaderCompleteness(now, st, snap.Log)
		}
	}

	// State machine safety: every committed index maps to one entry cluster-wide.
	done := v.checked[st.ID]
	if st.CommitIndex < done {
		done = st.CommitIndex // restart: re-verify the re-committed prefix
	}
	upper := st.CommitIndex
	if upper > lastIdx {
		upper = lastIdx
	}
	for i := done + 1; i <= upper; i++ {
		e := snap.Log[i]
		if prev, ok := v.committed[i]; ok {
			if prev.Term != e.Term || prev.Command != e.Command {
				v.add(now, StateMachineSafety, st.ID, st.CurrentTerm, i,
					"committed entry at index %d differs: (term %d, %q) vs (term %d, %q)",
					i, prev.Term, prev.Command, e.Term, e.Command)
			}
		} else {
			v.committed[i] = e
		}
	}
	v.checked[st.ID] = upper
}

func (v *Verifier) checkLeaderCompleteness(now time.Duration, st raft.NodeStatus, log []raft.LogEntry) {
	for idx, e := range v.committed {
		if e.Term >= st.CurrentTerm {
			continue // the property only covers entries committed in earlier terms
		}
		if idx >= uint64(len(log)) || log[idx].Term != e.Term || log[idx].Command != e.Command {
			v.add(now, LeaderCompleteness, st.ID, st.CurrentTerm, idx,
				"leader of term %d lacks committed entry %d (term %d)", st.CurrentTerm, idx, e.Term)
		}
	}
}

// PeriodicCheck runs the pairwise checks: log matching and agreement of the
// applied state machines.
func (v *Verifier) PeriodicCheck(now time.Duration, nodes []raft.Node) {
	snaps := make([]raft.NodeSnapshot, len(nodes))
	for i, n := range nodes {
		snaps[i] = n.Snapshot()
	}
	for i := 0; i < len(snaps); i++ {
		for j := i + 1; j < len(snaps); j++ {
			v.checkLogMatching(now, snaps[i], snaps[j])
			if snaps[i].Status.LastApplied == snaps[j].Status.LastApplied &&
				nodes[i].StateDigest() != nodes[j].StateDigest() {
				v.add(now, StateMachineSafety, snaps[i].Status.ID, snaps[i].Status.CurrentTerm,
					snaps[i].Status.LastApplied,
					"%s and %s applied up to index %d but their state machines differ",
					snaps[i].Status.ID, snaps[j].Status.ID, snaps[i].Status.LastApplied)
			}
		}
	}
}

// checkLogMatching: if two logs hold an entry with the same index and term,
// the logs must be identical up to that index.
func (v *Verifier) checkLogMatching(now time.Duration, a, b raft.NodeSnapshot) {
	limit := len(a.Log)
	if len(b.Log) < limit {
		limit = len(b.Log)
	}
	for i := limit - 1; i >= 1; i-- {
		if a.Log[i].Term != b.Log[i].Term {
			continue
		}
		for j := 1; j <= i; j++ {
			if a.Log[j].Term != b.Log[j].Term || a.Log[j].Command != b.Log[j].Command {
				v.add(now, LogMatching, a.Status.ID, a.Status.CurrentTerm, uint64(j),
					"%s and %s agree on (index %d, term %d) but differ at index %d",
					a.Status.ID, b.Status.ID, i, a.Log[i].Term, j)
				return
			}
		}
		return // highest common (index, term) verified; lower prefixes are covered
	}
}

// Report returns the structured result for the observations so far.
func (v *Verifier) Report() Report {
	status := "passed"
	if len(v.violations) > 0 {
		status = "failed"
	}
	return Report{
		ScenarioID:        v.scenario,
		Seed:              v.seed,
		InvariantsChecked: InvariantsChecked,
		Observations:      v.observations,
		Violations:        append([]Violation{}, v.violations...),
		Status:            status,
	}
}
