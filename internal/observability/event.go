// Package observability defines structured simulation events, a bounded
// in-memory recorder with a reproducibility hash, and metric helpers.
package observability

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"hash"
	"io"
	"sync"
)

// Event is the unified event schema used by the engine, the API and the
// experiment runner. Wall-clock time is deliberately absent so traces of the
// deterministic mode are byte-for-byte reproducible.
type Event struct {
	ID            uint64         `json:"event_id"`
	VirtualTimeMs float64        `json:"virtual_time_ms"`
	Type          string         `json:"type"`
	NodeID        string         `json:"node_id,omitempty"`
	Term          uint64         `json:"term"`
	LogIndex      uint64         `json:"log_index,omitempty"`
	MessageID     uint64         `json:"message_id,omitempty"`
	ScenarioID    string         `json:"scenario_id,omitempty"`
	Seed          int64          `json:"seed"`
	Metadata      map[string]any `json:"metadata,omitempty"`
}

// Recorder keeps the most recent events (bounded) and a running hash over
// every event ever recorded, including those already dropped.
type Recorder struct {
	mu       sync.Mutex
	events   []Event
	max      int
	nextID   uint64
	dropped  uint64
	scenario string
	seed     int64
	hasher   hash.Hash
}

// NewRecorder creates a recorder keeping at most max events (max <= 0 means 100000).
func NewRecorder(max int, scenario string, seed int64) *Recorder {
	if max <= 0 {
		max = 100000
	}
	return &Recorder{max: max, scenario: scenario, seed: seed, nextID: 1, hasher: sha256.New()}
}

// Record stamps and stores an event and returns the stored copy.
func (r *Recorder) Record(ev Event) Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	ev.ID = r.nextID
	r.nextID++
	ev.ScenarioID = r.scenario
	ev.Seed = r.seed
	if line, err := json.Marshal(ev); err == nil {
		r.hasher.Write(line)
	}
	if len(r.events) >= r.max {
		drop := r.max/10 + 1
		r.dropped += uint64(drop)
		r.events = append([]Event(nil), r.events[drop:]...)
	}
	r.events = append(r.events, ev)
	return ev
}

// Since returns up to limit events with ID greater than afterID.
func (r *Recorder) Since(afterID uint64, limit int) []Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.events) == 0 {
		return nil
	}
	start := 0
	if first := r.events[0].ID; afterID >= first {
		start = int(afterID - first + 1)
	}
	if start >= len(r.events) {
		return nil
	}
	end := len(r.events)
	if limit > 0 && start+limit < end {
		end = start + limit
	}
	return append([]Event(nil), r.events[start:end]...)
}

// All returns a copy of the retained events.
func (r *Recorder) All() []Event {
	return r.Since(0, 0)
}

func (r *Recorder) Len() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.events)
}

// Dropped is the number of old events evicted because of the size bound.
func (r *Recorder) Dropped() uint64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.dropped
}

// TraceHash fingerprints the full event history (including evicted events).
func (r *Recorder) TraceHash() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return hex.EncodeToString(r.hasher.Sum(nil))
}

// WriteJSONL writes the retained events, one JSON object per line.
func (r *Recorder) WriteJSONL(w io.Writer) error {
	enc := json.NewEncoder(w)
	for _, ev := range r.All() {
		if err := enc.Encode(ev); err != nil {
			return err
		}
	}
	return nil
}
