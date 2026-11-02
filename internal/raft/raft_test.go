package raft

import (
	"errors"
	"fmt"
	"testing"
	"time"
)

type harness struct {
	t       *testing.T
	now     time.Duration
	ids     []NodeID
	servers map[NodeID]*Server
	queue   []Message
	blocked map[NodeID]bool // messages to these nodes are dropped
}

func newHarness(t *testing.T, n int) *harness {
	t.Helper()
	h := &harness{t: t, servers: map[NodeID]*Server{}, blocked: map[NodeID]bool{}}
	for i := 1; i <= n; i++ {
		h.ids = append(h.ids, NodeID(fmt.Sprintf("n%d", i)))
	}
	for i, id := range h.ids {
		s, err := NewServer(Config{
			ID:                 id,
			Peers:              h.ids,
			Seed:               int64(i + 1),
			ElectionTimeoutMin: 150 * time.Millisecond,
			ElectionTimeoutMax: 300 * time.Millisecond,
			HeartbeatInterval:  50 * time.Millisecond,
		}, Env{
			Now:  func() time.Duration { return h.now },
			Send: func(m Message) { h.queue = append(h.queue, m) },
		})
		if err != nil {
			t.Fatalf("NewServer: %v", err)
		}
		h.servers[id] = s
	}
	return h
}

// pump delivers queued messages in FIFO order until the queue is empty.
func (h *harness) pump() {
	h.t.Helper()
	for i := 0; len(h.queue) > 0; i++ {
		if i > 100000 {
			h.t.Fatal("message pump did not terminate")
		}
		m := h.queue[0]
		h.queue = h.queue[1:]
		if h.blocked[m.To] {
			continue
		}
		h.servers[m.To].HandleMessage(m)
	}
}

// electN1 makes n1 the leader by letting only n1 time out.
func (h *harness) electN1() *Server {
	h.t.Helper()
	h.now = 400 * time.Millisecond
	h.servers["n1"].Tick()
	h.pump()
	if got := h.servers["n1"].Status().Role; got != "leader" {
		h.t.Fatalf("n1 role = %s, want leader", got)
	}
	return h.servers["n1"]
}

func (h *harness) heartbeat() {
	h.now += 60 * time.Millisecond
	h.servers["n1"].Tick()
	h.pump()
}

func TestNewServerInitialState(t *testing.T) {
	h := newHarness(t, 3)
	st := h.servers["n1"].Status()
	if st.Role != "follower" || st.CurrentTerm != 0 || st.CommitIndex != 0 || st.LastApplied != 0 || st.LogLength != 0 {
		t.Fatalf("unexpected initial status: %+v", st)
	}
}

func TestConfigValidation(t *testing.T) {
	base := Config{
		ID: "a", Peers: []NodeID{"a", "b", "c"},
		ElectionTimeoutMin: 150 * time.Millisecond, ElectionTimeoutMax: 300 * time.Millisecond,
		HeartbeatInterval: 50 * time.Millisecond,
	}
	if err := base.Validate(); err != nil {
		t.Fatalf("valid config rejected: %v", err)
	}
	dup := base
	dup.Peers = []NodeID{"a", "b", "b"}
	if dup.Validate() == nil {
		t.Error("duplicate peers accepted")
	}
	missing := base
	missing.Peers = []NodeID{"b", "c"}
	if missing.Validate() == nil {
		t.Error("config without self accepted")
	}
	slow := base
	slow.HeartbeatInterval = 200 * time.Millisecond
	if slow.Validate() == nil {
		t.Error("heartbeat >= election timeout accepted")
	}
}

func TestElectionTimeoutStartsElection(t *testing.T) {
	h := newHarness(t, 3)
	s := h.servers["n1"]
	s.Tick() // now = 0: nothing happens
	if s.Status().Role != "follower" {
		t.Fatal("node started an election before its timeout")
	}
	h.now = 301 * time.Millisecond
	s.Tick()
	st := s.Status()
	if st.Role != "candidate" || st.CurrentTerm != 1 || st.VotedFor != "n1" {
		t.Fatalf("after timeout: %+v", st)
	}
	if len(h.queue) != 2 {
		t.Fatalf("expected 2 RequestVote messages, got %d", len(h.queue))
	}
	for _, m := range h.queue {
		if m.Kind != MsgRequestVote || m.RequestVote.Term != 1 {
			t.Fatalf("unexpected message %+v", m)
		}
	}
}

func TestLeaderElectedByMajority(t *testing.T) {
	h := newHarness(t, 3)
	h.electN1()
	for _, id := range []NodeID{"n2", "n3"} {
		st := h.servers[id].Status()
		if st.Role != "follower" || st.CurrentTerm != 1 || st.LeaderID != "n1" {
			t.Errorf("%s: %+v", id, st)
		}
	}
}

func TestVoteOncePerTerm(t *testing.T) {
	h := newHarness(t, 3)
	s := h.servers["n1"]
	ask := func(from NodeID) bool {
		h.queue = nil
		s.HandleMessage(Message{From: from, To: "n1", Kind: MsgRequestVote,
			RequestVote: &RequestVoteRequest{Term: 1, CandidateID: from}})
		return h.queue[0].RequestVoteResponse.VoteGranted
	}
	if !ask("n2") {
		t.Fatal("first vote in the term was denied")
	}
	if ask("n3") {
		t.Fatal("second candidate received a vote in the same term")
	}
	if !ask("n2") {
		t.Fatal("repeated request from the same candidate should be granted again")
	}
}

func TestRejectCandidateWithStaleLog(t *testing.T) {
	h := newHarness(t, 3)
	s := h.servers["n1"]
	s.HandleMessage(Message{From: "n2", To: "n1", Kind: MsgAppendEntries, AppendEntries: &AppendEntriesRequest{
		Term: 2, LeaderID: "n2", Entries: []LogEntry{{Index: 1, Term: 2, Command: "SET a 1"}},
	}})
	h.queue = nil
	s.HandleMessage(Message{From: "n3", To: "n1", Kind: MsgRequestVote, RequestVote: &RequestVoteRequest{
		Term: 3, CandidateID: "n3", LastLogIndex: 5, LastLogTerm: 1,
	}})
	resp := h.queue[0].RequestVoteResponse
	if resp.VoteGranted {
		t.Fatal("vote granted to a candidate whose last log term is older")
	}
	if resp.Term != 3 || s.Status().CurrentTerm != 3 {
		t.Fatal("term was not updated to the newer term")
	}
}

func TestHigherTermMakesLeaderStepDown(t *testing.T) {
	h := newHarness(t, 3)
	leader := h.electN1()
	leader.HandleMessage(Message{From: "n2", To: "n1", Kind: MsgRequestVote,
		RequestVote: &RequestVoteRequest{Term: 99, CandidateID: "n2"}})
	st := leader.Status()
	if st.Role != "follower" || st.CurrentTerm != 99 {
		t.Fatalf("leader did not step down: %+v", st)
	}
}

func TestAppendEntriesRejectsStaleTerm(t *testing.T) {
	h := newHarness(t, 3)
	s := h.servers["n2"]
	s.HandleMessage(Message{From: "n1", To: "n2", Kind: MsgRequestVote, RequestVote: &RequestVoteRequest{Term: 5, CandidateID: "n1"}})
	h.queue = nil
	s.HandleMessage(Message{From: "n3", To: "n2", Kind: MsgAppendEntries, AppendEntries: &AppendEntriesRequest{Term: 2, LeaderID: "n3"}})
	resp := h.queue[0].AppendEntriesResp
	if resp.Success || resp.Term != 5 {
		t.Fatalf("stale AppendEntries accepted: %+v", resp)
	}
}

func TestAppendEntriesRejectsMissingPrevEntry(t *testing.T) {
	h := newHarness(t, 3)
	s := h.servers["n2"]
	s.HandleMessage(Message{From: "n1", To: "n2", Kind: MsgAppendEntries, AppendEntries: &AppendEntriesRequest{
		Term: 1, LeaderID: "n1", PrevLogIndex: 5, PrevLogTerm: 1,
	}})
	resp := h.queue[0].AppendEntriesResp
	if resp.Success || resp.ConflictIndex != 1 || resp.ConflictTerm != 0 {
		t.Fatalf("unexpected response: %+v", resp)
	}
}

func TestAppendEntriesTruncatesConflictingSuffix(t *testing.T) {
	h := newHarness(t, 3)
	s := h.servers["n2"]
	s.HandleMessage(Message{From: "n1", To: "n2", Kind: MsgAppendEntries, AppendEntries: &AppendEntriesRequest{
		Term: 1, LeaderID: "n1", Entries: []LogEntry{
			{Index: 1, Term: 1, Command: "SET a 1"},
			{Index: 2, Term: 1, Command: "SET b 2"},
		},
	}})
	if s.Status().LogLength != 2 {
		t.Fatal("setup failed")
	}
	s.HandleMessage(Message{From: "n3", To: "n2", Kind: MsgAppendEntries, AppendEntries: &AppendEntriesRequest{
		Term: 2, LeaderID: "n3", PrevLogIndex: 1, PrevLogTerm: 1,
		Entries: []LogEntry{{Index: 2, Term: 2, Command: "SET c 3"}},
	}})
	log := s.Snapshot().Log
	if len(log) != 3 || log[2].Term != 2 || log[2].Command != "SET c 3" {
		t.Fatalf("conflicting entry was not replaced: %+v", log)
	}
}

func TestStaleDuplicateAppendDoesNotTruncate(t *testing.T) {
	h := newHarness(t, 3)
	s := h.servers["n2"]
	full := &AppendEntriesRequest{Term: 1, LeaderID: "n1", Entries: []LogEntry{
		{Index: 1, Term: 1, Command: "SET a 1"}, {Index: 2, Term: 1, Command: "SET b 2"},
	}}
	s.HandleMessage(Message{From: "n1", To: "n2", Kind: MsgAppendEntries, AppendEntries: full})
	older := &AppendEntriesRequest{Term: 1, LeaderID: "n1", Entries: []LogEntry{{Index: 1, Term: 1, Command: "SET a 1"}}}
	s.HandleMessage(Message{From: "n1", To: "n2", Kind: MsgAppendEntries, AppendEntries: older})
	if s.Status().LogLength != 2 {
		t.Fatal("a delayed, shorter AppendEntries removed matching entries")
	}
}

func TestReplicationAndCommit(t *testing.T) {
	h := newHarness(t, 3)
	leader := h.electN1()
	idx, term, err := leader.Propose("SET x 1")
	if err != nil {
		t.Fatal(err)
	}
	if idx != 2 || term != 1 { // index 1 is the leader's no-op
		t.Fatalf("Propose returned index=%d term=%d", idx, term)
	}
	h.pump()
	if got := leader.Status().CommitIndex; got != 2 {
		t.Fatalf("leader commitIndex = %d, want 2", got)
	}
	h.heartbeat() // followers learn the new commit index
	for _, id := range h.ids {
		s := h.servers[id]
		st := s.Status()
		if st.CommitIndex != 2 || st.LastApplied != 2 {
			t.Errorf("%s: %+v", id, st)
		}
		if v, ok := s.sm.Get("x"); !ok || v != "1" {
			t.Errorf("%s: state machine missing x=1", id)
		}
	}
}

func TestCommitNeedsQuorum(t *testing.T) {
	h := newHarness(t, 3)
	leader := h.electN1()

	// The leader's no-op entry may already be committed during election.
	commitBefore := leader.Status().CommitIndex

	h.blocked["n2"], h.blocked["n3"] = true, true

	index, _, err := leader.Propose("SET x 1")
	if err != nil {
		t.Fatal(err)
	}

	h.pump()

	if got := leader.Status().CommitIndex; got != commitBefore {
		t.Fatalf(
			"entry committed without a quorum (commitIndex=%d, before=%d)",
			got,
			commitBefore,
		)
	}

	if _, ok := leader.CommittedEntry(index); ok {
		t.Fatalf("entry at index %d was committed without a quorum", index)
	}
}

// Figure 8 of the Raft paper: an entry from an earlier term must not be
// committed just because it is stored on a majority.
func TestCommitRequiresCurrentTermEntry(t *testing.T) {
	h := newHarness(t, 3)
	s := h.servers["n1"]
	s.currentTerm = 2
	s.role = Leader
	s.log = []LogEntry{{}, {Index: 1, Term: 1, Command: "SET a 1"}}
	s.matchIndex = map[NodeID]uint64{"n1": 1, "n2": 1}
	s.advanceCommit()
	if s.commitIndex != 0 {
		t.Fatal("committed an old-term entry by counting replicas")
	}
	s.log = append(s.log, LogEntry{Index: 2, Term: 2, Command: "SET b 2"})
	s.matchIndex = map[NodeID]uint64{"n1": 2, "n2": 2}
	s.advanceCommit()
	if s.commitIndex != 2 {
		t.Fatalf("commitIndex = %d, want 2 once a current-term entry is replicated", s.commitIndex)
	}
}

func TestLaggingFollowerIsRepaired(t *testing.T) {
	h := newHarness(t, 3)
	leader := h.electN1()
	h.blocked["n3"] = true
	for i := 0; i < 5; i++ {
		if _, _, err := leader.Propose(fmt.Sprintf("SET k%d v%d", i, i)); err != nil {
			t.Fatal(err)
		}
		h.pump()
	}
	delete(h.blocked, "n3")
	for i := 0; i < 5; i++ {
		h.heartbeat()
	}
	if got, want := h.servers["n3"].Status().LogLength, leader.Status().LogLength; got != want {
		t.Fatalf("n3 log length %d, leader %d", got, want)
	}
	if h.servers["n3"].Status().CommitIndex != leader.Status().CommitIndex {
		t.Fatal("n3 did not learn the commit index")
	}
}

func TestProposeErrors(t *testing.T) {
	h := newHarness(t, 3)
	if _, _, err := h.servers["n2"].Propose("SET a 1"); !errors.Is(err, ErrNotLeader) {
		t.Fatalf("follower Propose error = %v", err)
	}
	leader := h.electN1()
	if _, _, err := leader.Propose("DROP TABLE"); !errors.Is(err, ErrInvalidCommand) {
		t.Fatalf("invalid command error = %v", err)
	}
	if _, _, err := leader.Propose(NoopCommand); !errors.Is(err, ErrInvalidCommand) {
		t.Fatalf("reserved NOOP error = %v", err)
	}
}

func TestRestartKeepsPersistentStateOnly(t *testing.T) {
	h := newHarness(t, 3)
	leader := h.electN1()
	if _, _, err := leader.Propose("SET x 1"); err != nil {
		t.Fatal(err)
	}
	h.pump()
	h.heartbeat()
	f := h.servers["n2"]
	before := f.Status()
	f.Restart()
	after := f.Status()
	if after.CurrentTerm != before.CurrentTerm || after.LogLength != before.LogLength || after.VotedFor != before.VotedFor {
		t.Fatalf("persistent state changed: before=%+v after=%+v", before, after)
	}
	if after.Role != "follower" || after.CommitIndex != 0 || after.LastApplied != 0 {
		t.Fatalf("volatile state not reset: %+v", after)
	}
	if _, ok := f.sm.Get("x"); ok {
		t.Fatal("state machine survived the restart")
	}
}

func TestParseCommand(t *testing.T) {
	good := map[string]Command{
		`SET name "consensuslab"`: {Op: "SET", Key: "name", Value: "consensuslab"},
		"SET a b c":               {Op: "SET", Key: "a", Value: "b c"},
		"DELETE status":           {Op: "DELETE", Key: "status"},
		"NOOP":                    {Op: "NOOP"},
	}
	for raw, want := range good {
		got, err := ParseCommand(raw)
		if err != nil || got != want {
			t.Errorf("ParseCommand(%q) = %+v, %v; want %+v", raw, got, err, want)
		}
	}
	for _, raw := range []string{"", "SET", "SET a", "DELETE", "DELETE a b", "GET a", `SET a "unterminated`} {
		if _, err := ParseCommand(raw); !errors.Is(err, ErrInvalidCommand) {
			t.Errorf("ParseCommand(%q) error = %v, want ErrInvalidCommand", raw, err)
		}
	}
}

func TestKVStoreDigestIgnoresInsertionOrder(t *testing.T) {
	a, b := NewKVStore(), NewKVStore()
	a.Apply(LogEntry{Command: "SET x 1"})
	a.Apply(LogEntry{Command: "SET y 2"})
	b.Apply(LogEntry{Command: "SET y 2"})
	b.Apply(LogEntry{Command: "SET x 1"})
	if a.Digest() != b.Digest() {
		t.Fatal("digest depends on insertion order")
	}
	b.Apply(LogEntry{Command: "DELETE x"})
	if a.Digest() == b.Digest() {
		t.Fatal("digest did not change after a delete")
	}
}
