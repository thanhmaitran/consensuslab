package sim

import (
	"errors"
	"fmt"
	"hash/fnv"
	"time"

	"github.com/thanhmaitran/consensuslab/internal/network"
	"github.com/thanhmaitran/consensuslab/internal/observability"
	"github.com/thanhmaitran/consensuslab/internal/raft"
	"github.com/thanhmaitran/consensuslab/internal/verify"
)

const (
	periodicCheckEvery  = 50
	clientRetryInterval = 20 * time.Millisecond
	logTailSize         = 32
)

// LeaderElection records one LeaderElected observation.
type LeaderElection struct {
	At   time.Duration
	Node raft.NodeID
	Term uint64
}

// Metrics are counters and samples collected while the engine runs.
type Metrics struct {
	CommandsSubmitted int
	CommandsAccepted  int
	CommandsCommitted int
	CommandsLost      int
	CommandsRejected  int
	CommitLatencies   []time.Duration // accept-to-commit, one per committed command
	FirstAccept       time.Duration
	LastCommit        time.Duration
	Elections         []LeaderElection
	ElectionsStarted  int
}

// Input is one externally supplied action, kept for replay and failure reports.
type Input struct {
	AtMs    float64 `json:"at_ms"`
	Kind    string  `json:"kind"`
	NodeID  string  `json:"node_id,omitempty"`
	Command string  `json:"command,omitempty"`
	Detail  string  `json:"detail,omitempty"`
}

type pendingCommand struct {
	index      uint64
	term       uint64
	acceptedAt time.Duration
}

// Engine runs the whole cluster in a single goroutine on virtual time.
// It is not safe for concurrent use; callers (e.g. the API) must serialise.
type Engine struct {
	Now      time.Duration
	Seed     int64
	Sequence uint64

	Nodes    map[raft.NodeID]raft.Node
	Network  *network.Network
	Queue    *EventQueue
	Recorder *observability.Recorder
	Verifier *verify.Verifier

	// OnStep, if set, runs after every processed event.
	OnStep func()

	cfg       Config
	order     []raft.NodeID
	gen       map[raft.NodeID]uint64
	down      map[raft.NodeID]bool
	nextMsgID uint64
	cmdSeq    uint64
	steps     uint64
	pending   []pendingCommand
	metrics   Metrics
	inputs    []Input
}

func nodeSeed(seed int64, id raft.NodeID) int64 {
	h := fnv.New64a()
	h.Write([]byte(id))
	return seed ^ int64(h.Sum64()>>1)
}

// NewEngine builds the cluster and schedules the first tick of every node.
func NewEngine(cfg Config) (*Engine, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	net, err := network.New(cfg.Network, cfg.Seed^0x5DEECE66D)
	if err != nil {
		return nil, err
	}
	e := &Engine{
		Seed:     cfg.Seed,
		Nodes:    make(map[raft.NodeID]raft.Node, cfg.NodeCount),
		Network:  net,
		Queue:    NewEventQueue(),
		Recorder: observability.NewRecorder(cfg.MaxEvents, cfg.ScenarioID, cfg.Seed),
		Verifier: verify.New(cfg.ScenarioID, cfg.Seed),
		cfg:      cfg,
		gen:      make(map[raft.NodeID]uint64),
		down:     make(map[raft.NodeID]bool),
	}
	e.Verifier.OnViolation = func(v verify.Violation) {
		e.record("SafetyViolationDetected", raft.NodeID(v.NodeID), v.Term, v.Index, 0,
			map[string]any{"invariant": v.Invariant, "detail": v.Detail})
	}

	for i := 1; i <= cfg.NodeCount; i++ {
		e.order = append(e.order, NodeName(i))
	}
	for i, id := range e.order {
		id := id
		srv, err := raft.NewServer(raft.Config{
			ID:                 id,
			Peers:              e.order,
			Seed:               nodeSeed(cfg.Seed, id),
			ElectionTimeoutMin: cfg.ElectionTimeoutMin,
			ElectionTimeoutMax: cfg.ElectionTimeoutMax,
			HeartbeatInterval:  cfg.HeartbeatInterval,
			VerboseEvents:      cfg.VerboseEvents,
		}, raft.Env{
			Now:  func() time.Duration { return e.Now },
			Send: func(m raft.Message) { e.Send(m) },
			Emit: func(typ string, term, index uint64, meta map[string]any) {
				e.record(typ, id, term, index, 0, meta)
			},
		})
		if err != nil {
			return nil, err
		}
		e.Nodes[id] = srv
		e.record("NodeStarted", id, 0, 0, 0, nil)
		e.schedule(Event{At: time.Duration(i) * time.Millisecond, Kind: EventNodeTick, To: id})
	}
	return e, nil
}

var _ network.Transport = (*Engine)(nil)

// Send implements network.Transport: the network decides the message's fate.
func (e *Engine) Send(m raft.Message) {
	e.nextMsgID++
	m.ID = e.nextMsgID
	d := e.Network.Route(m.From, m.To)
	if !d.Deliver {
		if e.cfg.VerboseEvents {
			e.record("MessageDropped", m.From, 0, 0, m.ID,
				map[string]any{"to": string(m.To), "kind": m.Kind.String(), "reason": d.Reason})
		}
		return
	}
	msg := m
	e.schedule(Event{At: e.Now + d.Delay, Kind: EventDeliverMessage, From: m.From, To: m.To, Msg: &msg})
}

func (e *Engine) schedule(ev Event) {
	e.Sequence++
	ev.Seq = e.Sequence
	e.Queue.Schedule(ev)
}

func (e *Engine) record(typ string, node raft.NodeID, term, index, msgID uint64, meta map[string]any) {
	e.Recorder.Record(observability.Event{
		VirtualTimeMs: float64(e.Now) / float64(time.Millisecond),
		Type:          typ,
		NodeID:        string(node),
		Term:          term,
		LogIndex:      index,
		MessageID:     msgID,
		Metadata:      meta,
	})
	switch typ {
	case "LeaderElected":
		e.metrics.Elections = append(e.metrics.Elections, LeaderElection{At: e.Now, Node: node, Term: term})
	case "ElectionStarted":
		e.metrics.ElectionsStarted++
	}
}

// ------------------------------------------------------------ accessors

func (e *Engine) Config() Config             { return e.cfg }
func (e *Engine) NodeIDs() []raft.NodeID     { return append([]raft.NodeID(nil), e.order...) }
func (e *Engine) IsDown(id raft.NodeID) bool { return e.down[id] }
func (e *Engine) Steps() uint64              { return e.steps }
func (e *Engine) Metrics() Metrics           { return e.metrics }
func (e *Engine) Inputs() []Input            { return append([]Input(nil), e.inputs...) }
func (e *Engine) Quorum() int                { return len(e.order)/2 + 1 }

// SetMaxVirtualTime lets an interactive session extend the run limit.
func (e *Engine) SetMaxVirtualTime(d time.Duration) { e.cfg.MaxVirtualTime = d }

// Leader returns the up node that is leader in the highest term, if any.
func (e *Engine) Leader() raft.NodeID {
	var best raft.NodeID
	var bestTerm uint64
	for _, id := range e.order {
		if e.down[id] {
			continue
		}
		st := e.Nodes[id].Status()
		if st.Role == raft.Leader.String() && (best == "" || st.CurrentTerm > bestTerm) {
			best, bestTerm = id, st.CurrentTerm
		}
	}
	return best
}

// ClusterStable reports whether the recovery condition holds: no partition,
// a quorum of nodes is up, exactly one leader exists, it has committed an
// entry of its own term, and every up node has the same term, log and commit index.
func (e *Engine) ClusterStable() bool {
	if e.Network.Partitioned() {
		return false
	}
	leader := e.Leader()
	if leader == "" {
		return false
	}
	ls := e.Nodes[leader].Status()
	if ls.CommitIndex == 0 || int(ls.CommitIndex) != ls.LogLength {
		return false
	}
	if entry, ok := e.Nodes[leader].CommittedEntry(ls.CommitIndex); !ok || entry.Term != ls.CurrentTerm {
		return false
	}
	up := 0
	for _, id := range e.order {
		if e.down[id] {
			continue
		}
		up++
		st := e.Nodes[id].Status()
		if st.CurrentTerm != ls.CurrentTerm || st.CommitIndex != ls.CommitIndex || st.LogLength != ls.LogLength {
			return false
		}
		if id != leader && st.Role == raft.Leader.String() {
			return false
		}
	}
	return up >= e.Quorum()
}

// ------------------------------------------------------------ scheduling

func (e *Engine) clamp(at time.Duration) time.Duration {
	if at < e.Now {
		return e.Now
	}
	return at
}

func (e *Engine) addInput(at time.Duration, kind string, node raft.NodeID, command, detail string) {
	e.inputs = append(e.inputs, Input{
		AtMs: float64(at) / float64(time.Millisecond), Kind: kind,
		NodeID: string(node), Command: command, Detail: detail,
	})
}

// ErrUnknownNode is returned when a request names a node that does not exist.
var ErrUnknownNode = errors.New("sim: unknown node")

func (e *Engine) checkNode(id raft.NodeID) error {
	if _, ok := e.Nodes[id]; !ok {
		return fmt.Errorf("%w %q", ErrUnknownNode, id)
	}
	return nil
}

// ScheduleCrash crashes id at the given time. An empty id means "whoever is
// leader at that moment". If restartAfter > 0 the node restarts later.
func (e *Engine) ScheduleCrash(at time.Duration, id raft.NodeID, restartAfter time.Duration) error {
	if id != "" {
		if err := e.checkNode(id); err != nil {
			return err
		}
	}
	at = e.clamp(at)
	e.addInput(at, "crash", id, "", restartAfter.String())
	e.schedule(Event{At: at, Kind: EventCrashNode, To: id, RestartAfter: restartAfter})
	return nil
}

func (e *Engine) ScheduleRestart(at time.Duration, id raft.NodeID) error {
	if err := e.checkNode(id); err != nil {
		return err
	}
	at = e.clamp(at)
	e.addInput(at, "restart", id, "", "")
	e.schedule(Event{At: at, Kind: EventRestartNode, To: id})
	return nil
}

// SchedulePartitionSizes splits the nodes into consecutive groups of the given
// sizes. With isolateLeader the leader (resolved when the event fires) is
// placed in the last group.
func (e *Engine) SchedulePartitionSizes(at time.Duration, sizes []int, isolateLeader bool) error {
	sum := 0
	for _, s := range sizes {
		if s < 1 {
			return errors.New("sim: partition group sizes must be positive")
		}
		sum += s
	}
	if sum != len(e.order) {
		return fmt.Errorf("sim: partition sizes sum to %d but the cluster has %d nodes", sum, len(e.order))
	}
	at = e.clamp(at)
	e.addInput(at, "partition", "", "", fmt.Sprint(sizes))
	e.schedule(Event{At: at, Kind: EventPartition, Sizes: append([]int(nil), sizes...), IsolateLeader: isolateLeader})
	return nil
}

func (e *Engine) SchedulePartitionGroups(at time.Duration, groups [][]raft.NodeID) error {
	for _, g := range groups {
		for _, id := range g {
			if err := e.checkNode(id); err != nil {
				return err
			}
		}
	}
	at = e.clamp(at)
	e.addInput(at, "partition", "", "", fmt.Sprint(groups))
	cp := make([][]raft.NodeID, len(groups))
	for i, g := range groups {
		cp[i] = append([]raft.NodeID(nil), g...)
	}
	e.schedule(Event{At: at, Kind: EventPartition, Groups: cp})
	return nil
}

func (e *Engine) ScheduleHeal(at time.Duration) {
	at = e.clamp(at)
	e.addInput(at, "heal", "", "", "")
	e.schedule(Event{At: at, Kind: EventHeal})
}

func (e *Engine) ScheduleNetwork(at time.Duration, cfg network.Config) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	at = e.clamp(at)
	e.addInput(at, "network", "", "", fmt.Sprintf("%+v", cfg))
	e.schedule(Event{At: at, Kind: EventSetNetwork, Net: &cfg})
	return nil
}

// ScheduleCommand submits a client command at the given time. An empty `to`
// targets the current leader and retries until one exists.
func (e *Engine) ScheduleCommand(at time.Duration, to raft.NodeID, command string) (uint64, error) {
	if err := raft.ValidateClientCommand(command); err != nil {
		return 0, err
	}
	if to != "" {
		if err := e.checkNode(to); err != nil {
			return 0, err
		}
	}
	at = e.clamp(at)
	e.cmdSeq++
	e.addInput(at, "command", to, command, "")
	e.schedule(Event{At: at, Kind: EventClientCommand, To: to, Command: command, CommandID: e.cmdSeq})
	return e.cmdSeq, nil
}

// Immediate variants used by the interactive API: schedule at Now and process.

func (e *Engine) Crash(id raft.NodeID) error {
	if id == "" {
		return errors.New("sim: node id required")
	}
	if err := e.ScheduleCrash(e.Now, id, 0); err != nil {
		return err
	}
	e.ProcessDue()
	return nil
}

func (e *Engine) Restart(id raft.NodeID) error {
	if err := e.ScheduleRestart(e.Now, id); err != nil {
		return err
	}
	e.ProcessDue()
	return nil
}

func (e *Engine) Partition(groups [][]raft.NodeID) error {
	if err := e.SchedulePartitionGroups(e.Now, groups); err != nil {
		return err
	}
	e.ProcessDue()
	return nil
}

func (e *Engine) Heal() {
	e.ScheduleHeal(e.Now)
	e.ProcessDue()
}

func (e *Engine) SetNetwork(cfg network.Config) error {
	if err := e.ScheduleNetwork(e.Now, cfg); err != nil {
		return err
	}
	e.ProcessDue()
	return nil
}

func (e *Engine) Submit(to raft.NodeID, command string) (uint64, error) {
	id, err := e.ScheduleCommand(e.Now, to, command)
	if err != nil {
		return 0, err
	}
	e.ProcessDue()
	return id, nil
}

// ------------------------------------------------------------ running

// Step processes the earliest event. It returns false when the queue is empty
// or the next event lies beyond MaxVirtualTime.
func (e *Engine) Step() bool {
	next, ok := e.Queue.Peek()
	if !ok || next.At > e.cfg.MaxVirtualTime {
		return false
	}
	ev, _ := e.Queue.Next()
	if ev.At > e.Now {
		e.Now = ev.At
	}
	touched := e.process(ev)
	e.steps++
	if touched != "" {
		e.Verifier.ObserveNode(e.Now, e.Nodes[touched])
		e.checkPending(touched)
	}
	if e.steps%periodicCheckEvery == 0 {
		e.periodicCheck()
	}
	if e.OnStep != nil {
		e.OnStep()
	}
	return true
}

// RunUntil processes all events due at or before t and advances the clock to t.
func (e *Engine) RunUntil(t time.Duration) int {
	n := 0
	for {
		next, ok := e.Queue.Peek()
		if !ok || next.At > t || next.At > e.cfg.MaxVirtualTime {
			break
		}
		if !e.Step() {
			break
		}
		n++
	}
	if t > e.Now && t <= e.cfg.MaxVirtualTime {
		e.Now = t
	}
	return n
}

func (e *Engine) RunFor(d time.Duration) int { return e.RunUntil(e.Now + d) }

// ProcessDue handles events already due at the current time.
func (e *Engine) ProcessDue() int { return e.RunUntil(e.Now) }

// Finish runs the final pairwise checks and returns the verifier report.
func (e *Engine) Finish() verify.Report {
	e.periodicCheck()
	return e.Verifier.Report()
}

func (e *Engine) periodicCheck() {
	nodes := make([]raft.Node, 0, len(e.order))
	for _, id := range e.order {
		nodes = append(nodes, e.Nodes[id])
	}
	e.Verifier.PeriodicCheck(e.Now, nodes)
}

func (e *Engine) scheduleTick(id raft.NodeID) {
	e.schedule(Event{At: e.Now + e.cfg.TickInterval, Kind: EventNodeTick, To: id, Gen: e.gen[id]})
}

// process executes one event and returns the node whose state it changed, if any.
func (e *Engine) process(ev Event) raft.NodeID {
	switch ev.Kind {
	case EventNodeTick:
		if e.down[ev.To] || ev.Gen != e.gen[ev.To] {
			return ""
		}
		e.Nodes[ev.To].Tick()
		e.scheduleTick(ev.To)
		return ev.To

	case EventDeliverMessage:
		if e.down[ev.To] {
			e.Network.RecordDroppedDown()
			return ""
		}
		if !e.Network.Reachable(ev.From, ev.To) {
			e.Network.RecordDroppedPartition()
			return ""
		}
		e.Network.RecordDelivered()
		e.Nodes[ev.To].HandleMessage(*ev.Msg)
		return ev.To

	case EventCrashNode:
		id := ev.To
		if id == "" {
			id = e.Leader()
		}
		if id == "" || e.down[id] {
			e.record("FaultSkipped", id, 0, 0, 0, map[string]any{"fault": "crash", "reason": "no eligible node"})
			return ""
		}
		e.down[id] = true
		e.gen[id]++
		st := e.Nodes[id].Status()
		e.record("NodeCrashed", id, st.CurrentTerm, 0, 0, map[string]any{"role": st.Role})
		if ev.RestartAfter > 0 {
			e.schedule(Event{At: e.Now + ev.RestartAfter, Kind: EventRestartNode, To: id})
		}
		return ""

	case EventRestartNode:
		id := ev.To
		if !e.down[id] {
			e.record("FaultSkipped", id, 0, 0, 0, map[string]any{"fault": "restart", "reason": "node is not down"})
			return ""
		}
		e.down[id] = false
		e.gen[id]++
		e.Nodes[id].Restart()
		e.scheduleTick(id)
		e.record("NodeRestarted", id, e.Nodes[id].Status().CurrentTerm, 0, 0, nil)
		return id

	case EventPartition:
		e.applyPartition(ev)
		return ""

	case EventHeal:
		e.Network.Heal()
		e.record("NetworkPartitionHealed", "", 0, 0, 0, nil)
		return ""

	case EventSetNetwork:
		if ev.Net != nil {
			if err := e.Network.SetConfig(*ev.Net); err == nil {
				e.record("NetworkConfigChanged", "", 0, 0, 0, map[string]any{
					"base_delay_ms":    float64(ev.Net.BaseDelay) / float64(time.Millisecond),
					"jitter_ms":        float64(ev.Net.Jitter) / float64(time.Millisecond),
					"loss_probability": ev.Net.LossProbability,
				})
			}
		}
		return ""

	case EventClientCommand:
		return e.handleClient(ev)
	}
	return ""
}

func (e *Engine) applyPartition(ev Event) {
	groups := ev.Groups
	if groups == nil {
		nodes := make([]raft.NodeID, 0, len(e.order))
		leader := raft.NodeID("")
		if ev.IsolateLeader {
			leader = e.Leader()
		}
		for _, id := range e.order {
			if id != leader {
				nodes = append(nodes, id)
			}
		}
		if leader != "" {
			nodes = append(nodes, leader)
		}
		pos := 0
		for _, size := range ev.Sizes {
			groups = append(groups, append([]raft.NodeID(nil), nodes[pos:pos+size]...))
			pos += size
		}
	}
	if err := e.Network.SetPartition(groups); err != nil {
		e.record("FaultSkipped", "", 0, 0, 0, map[string]any{"fault": "partition", "reason": err.Error()})
		return
	}
	e.record("NetworkPartitionCreated", "", 0, 0, 0, map[string]any{"groups": groupsToStrings(groups)})
}

func groupsToStrings(groups [][]raft.NodeID) [][]string {
	out := make([][]string, len(groups))
	for i, g := range groups {
		out[i] = make([]string, len(g))
		for j, id := range g {
			out[i][j] = string(id)
		}
	}
	return out
}

func (e *Engine) handleClient(ev Event) raft.NodeID {
	if ev.Attempt == 0 {
		e.metrics.CommandsSubmitted++
	}
	explicit := ev.To != ""
	target := ev.To
	if !explicit {
		target = e.Leader()
	}
	if target == "" || e.down[target] {
		return e.retryOrReject(ev, explicit)
	}
	index, term, err := e.Nodes[target].Propose(ev.Command)
	switch {
	case err == nil:
		e.metrics.CommandsAccepted++
		if e.metrics.CommandsAccepted == 1 {
			e.metrics.FirstAccept = e.Now
		}
		e.pending = append(e.pending, pendingCommand{index: index, term: term, acceptedAt: e.Now})
		e.record("ClientCommandAccepted", target, term, index, 0,
			map[string]any{"command_id": ev.CommandID, "command": ev.Command})
		return target
	case errors.Is(err, raft.ErrNotLeader):
		return e.retryOrReject(ev, explicit)
	default:
		e.metrics.CommandsRejected++
		e.record("ClientCommandRejected", target, 0, 0, 0,
			map[string]any{"command_id": ev.CommandID, "reason": err.Error()})
		return ""
	}
}

// retryOrReject re-queues a command aimed at "the leader"; a command aimed at
// a specific node is rejected instead.
func (e *Engine) retryOrReject(ev Event, explicit bool) raft.NodeID {
	if explicit {
		e.metrics.CommandsRejected++
		e.record("ClientCommandRejected", ev.To, 0, 0, 0,
			map[string]any{"command_id": ev.CommandID, "reason": "target is not an available leader"})
		return ""
	}
	ev.Attempt++
	ev.At = e.Now + clientRetryInterval
	e.schedule(ev)
	return ""
}

// checkPending resolves accepted commands once some node has committed their index.
func (e *Engine) checkPending(id raft.NodeID) {
	if len(e.pending) == 0 {
		return
	}
	node := e.Nodes[id]
	kept := e.pending[:0]
	for _, p := range e.pending {
		entry, ok := node.CommittedEntry(p.index)
		switch {
		case !ok:
			kept = append(kept, p)
		case entry.Term == p.term:
			latency := e.Now - p.acceptedAt
			e.metrics.CommandsCommitted++
			e.metrics.CommitLatencies = append(e.metrics.CommitLatencies, latency)
			e.metrics.LastCommit = e.Now
			e.record("ClientCommandCommitted", id, p.term, p.index, 0,
				map[string]any{"latency_ms": float64(latency) / float64(time.Millisecond)})
		default:
			e.metrics.CommandsLost++
			e.record("ClientCommandLost", id, p.term, p.index, 0,
				map[string]any{"reason": "entry was overwritten after a leader change"})
		}
	}
	e.pending = kept
}

// ------------------------------------------------------------ status view

// NodeView is the read-only per-node view served to dashboards.
type NodeView struct {
	raft.NodeStatus
	Down    bool            `json:"down"`
	LogTail []raft.LogEntry `json:"logTail"`
}

// MetricsView is the JSON form of the engine counters.
type MetricsView struct {
	CommandsSubmitted int `json:"commandsSubmitted"`
	CommandsAccepted  int `json:"commandsAccepted"`
	CommandsCommitted int `json:"commandsCommitted"`
	CommandsLost      int `json:"commandsLost"`
	CommandsRejected  int `json:"commandsRejected"`
	ElectionsStarted  int `json:"electionsStarted"`
	LeadersElected    int `json:"leadersElected"`
}

// NetworkView is the JSON form of the current network settings.
type NetworkView struct {
	BaseDelayMs     float64 `json:"baseDelayMs"`
	JitterMs        float64 `json:"jitterMs"`
	LossProbability float64 `json:"lossProbability"`
}

// ClusterStatus is a consistent snapshot of the whole simulation.
type ClusterStatus struct {
	ScenarioID    string        `json:"scenarioId"`
	Seed          int64         `json:"seed"`
	NowMs         float64       `json:"nowMs"`
	Steps         uint64        `json:"steps"`
	NodeCount     int           `json:"nodeCount"`
	Quorum        int           `json:"quorum"`
	Leader        string        `json:"leader"`
	Term          uint64        `json:"term"`
	Nodes         []NodeView    `json:"nodes"`
	Partition     [][]string    `json:"partition"`
	Network       network.Stats `json:"network"`
	NetworkConfig NetworkView   `json:"networkConfig"`
	Violations    int           `json:"violations"`
	Metrics       MetricsView   `json:"metrics"`
	TraceHash     string        `json:"traceHash"`
}

// Status returns a snapshot suitable for JSON encoding.
func (e *Engine) Status() ClusterStatus {
	st := ClusterStatus{
		ScenarioID: e.cfg.ScenarioID,
		Seed:       e.Seed,
		NowMs:      float64(e.Now) / float64(time.Millisecond),
		Steps:      e.steps,
		NodeCount:  len(e.order),
		Quorum:     e.Quorum(),
		Leader:     string(e.Leader()),
		Partition:  groupsToStrings(e.Network.Groups()),
		Network:    e.Network.Stats(),
		Violations: e.Verifier.ViolationCount(),
		TraceHash:  e.Recorder.TraceHash(),
	}
	nc := e.Network.Config()
	st.NetworkConfig = NetworkView{
		BaseDelayMs:     float64(nc.BaseDelay) / float64(time.Millisecond),
		JitterMs:        float64(nc.Jitter) / float64(time.Millisecond),
		LossProbability: nc.LossProbability,
	}
	m := e.metrics
	st.Metrics = MetricsView{
		CommandsSubmitted: m.CommandsSubmitted,
		CommandsAccepted:  m.CommandsAccepted,
		CommandsCommitted: m.CommandsCommitted,
		CommandsLost:      m.CommandsLost,
		CommandsRejected:  m.CommandsRejected,
		ElectionsStarted:  m.ElectionsStarted,
		LeadersElected:    len(m.Elections),
	}
	for _, id := range e.order {
		snap := e.Nodes[id].Snapshot()
		if snap.Status.CurrentTerm > st.Term {
			st.Term = snap.Status.CurrentTerm
		}
		tail := snap.Log[1:]
		if len(tail) > logTailSize {
			tail = tail[len(tail)-logTailSize:]
		}
		st.Nodes = append(st.Nodes, NodeView{
			NodeStatus: snap.Status,
			Down:       e.down[id],
			LogTail:    append([]raft.LogEntry{}, tail...),
		})
	}
	return st
}
