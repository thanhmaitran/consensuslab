// Package sim contains the deterministic simulation engine: a virtual clock,
// an ordered event queue and the node lifecycle.
package sim

import (
	"container/heap"
	"time"

	"github.com/thanhmaitran/consensuslab/internal/network"
	"github.com/thanhmaitran/consensuslab/internal/raft"
)

// EventKind identifies what the engine must do when an event becomes due.
type EventKind uint8

const (
	EventDeliverMessage EventKind = iota
	EventNodeTick
	EventCrashNode
	EventRestartNode
	EventClientCommand
	EventPartition
	EventHeal
	EventSetNetwork
)

// Event is one scheduled occurrence. Only the fields relevant to Kind are used.
type Event struct {
	At   time.Duration
	Seq  uint64 // assigned by the engine; breaks ties between equal timestamps
	Kind EventKind
	Gen  uint64 // node lifecycle generation; stale ticks are discarded

	From raft.NodeID
	To   raft.NodeID
	Msg  *raft.Message

	Command   string
	CommandID uint64
	Attempt   int

	Sizes         []int           // partition group sizes (leader placement is dynamic)
	Groups        [][]raft.NodeID // explicit partition groups
	IsolateLeader bool            // place the current leader in the last group
	RestartAfter  time.Duration   // for crashes: schedule a restart after this delay
	Net           *network.Config // for EventSetNetwork
}

// EventQueue is a min-heap ordered by (At, Seq).
type EventQueue []Event

func (q EventQueue) Len() int { return len(q) }

func (q EventQueue) Less(i, j int) bool {
	if q[i].At != q[j].At {
		return q[i].At < q[j].At
	}
	return q[i].Seq < q[j].Seq
}

func (q EventQueue) Swap(i, j int) { q[i], q[j] = q[j], q[i] }

func (q *EventQueue) Push(x any) { *q = append(*q, x.(Event)) }

func (q *EventQueue) Pop() any {
	old := *q
	n := len(old)
	item := old[n-1]
	*q = old[:n-1]
	return item
}

func NewEventQueue() *EventQueue {
	q := &EventQueue{}
	heap.Init(q)
	return q
}

// Schedule adds an event. The caller is responsible for setting Seq.
func (q *EventQueue) Schedule(e Event) { heap.Push(q, e) }

// Next removes and returns the earliest event.
func (q *EventQueue) Next() (Event, bool) {
	if q.Len() == 0 {
		return Event{}, false
	}
	return heap.Pop(q).(Event), true
}

// Peek returns the earliest event without removing it.
func (q *EventQueue) Peek() (Event, bool) {
	if q.Len() == 0 {
		return Event{}, false
	}
	return (*q)[0], true
}
