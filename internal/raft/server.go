package raft

import (
	"errors"
	"math/rand"
	"time"
)

// maxEntriesPerMessage bounds the size of a single AppendEntries message.
const maxEntriesPerMessage = 64

// Server is a Raft node with a fixed membership. It is not safe for
// concurrent use: the simulation engine serialises all calls.
type Server struct {
	cfg Config
	env Env
	rng *rand.Rand

	// Persistent state (survives Restart).
	currentTerm uint64
	votedFor    NodeID
	log         []LogEntry // log[0] is the sentinel (index 0, term 0)

	// Volatile state.
	commitIndex       uint64
	lastApplied       uint64
	role              Role
	leaderID          NodeID
	electionDeadline  time.Duration
	heartbeatDeadline time.Duration
	votes             map[NodeID]bool
	nextIndex         map[NodeID]uint64
	matchIndex        map[NodeID]uint64
	sm                *KVStore
}

var _ Node = (*Server)(nil)

// NewServer creates a follower with an empty log.
func NewServer(cfg Config, env Env) (*Server, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if env.Now == nil || env.Send == nil {
		return nil, errors.New("raft: Env.Now and Env.Send are required")
	}
	s := &Server{
		cfg: cfg,
		env: env,
		rng: rand.New(rand.NewSource(cfg.Seed)),
		log: []LogEntry{{Index: 0, Term: 0}},
		sm:  NewKVStore(),
	}
	s.resetElectionDeadline()
	return s, nil
}

func (s *Server) ID() NodeID { return s.cfg.ID }

func (s *Server) Status() NodeStatus {
	return NodeStatus{
		ID:          s.cfg.ID,
		Role:        s.role.String(),
		CurrentTerm: s.currentTerm,
		CommitIndex: s.commitIndex,
		LastApplied: s.lastApplied,
		LogLength:   len(s.log) - 1,
		LeaderID:    s.leaderID,
		VotedFor:    s.votedFor,
	}
}

func (s *Server) Snapshot() NodeSnapshot {
	return NodeSnapshot{
		Status: s.Status(),
		Log:    append([]LogEntry(nil), s.log...),
	}
}

func (s *Server) CommittedEntry(index uint64) (LogEntry, bool) {
	if index == 0 || index > s.commitIndex {
		return LogEntry{}, false
	}
	return s.log[index], true
}

func (s *Server) StateDigest() string { return s.sm.Digest() }

// Restart keeps term, vote and log, and resets everything else.
func (s *Server) Restart() {
	s.role = Follower
	s.leaderID = ""
	s.commitIndex = 0
	s.lastApplied = 0
	s.votes = nil
	s.nextIndex = nil
	s.matchIndex = nil
	s.sm = NewKVStore()
	s.resetElectionDeadline()
}

// Tick lets the node react to the passage of time.
func (s *Server) Tick() {
	now := s.env.Now()
	if s.role == Leader {
		if now >= s.heartbeatDeadline {
			s.broadcastAppend()
		}
		return
	}
	if now >= s.electionDeadline {
		s.startElection()
	}
}

// Propose appends a client command to the leader's log.
func (s *Server) Propose(command string) (uint64, uint64, error) {
	if s.role != Leader {
		return 0, 0, ErrNotLeader
	}
	if err := ValidateClientCommand(command); err != nil {
		return 0, 0, err
	}
	index := s.appendLocal(command)
	s.emit("ClientCommandAppended", index, map[string]any{"command": command})
	s.broadcastAppend()
	s.advanceCommit()
	return index, s.currentTerm, nil
}

// HandleMessage processes one delivered message.
func (s *Server) HandleMessage(m Message) {
	if m.To != s.cfg.ID {
		return
	}
	switch m.Kind {
	case MsgRequestVote:
		if m.RequestVote != nil {
			s.handleRequestVote(m.From, m.RequestVote)
		}
	case MsgRequestVoteResponse:
		if m.RequestVoteResponse != nil {
			s.handleRequestVoteResponse(m.From, m.RequestVoteResponse)
		}
	case MsgAppendEntries:
		if m.AppendEntries != nil {
			s.handleAppendEntries(m.From, m.AppendEntries)
		}
	case MsgAppendEntriesResponse:
		if m.AppendEntriesResp != nil {
			s.handleAppendEntriesResponse(m.From, m.AppendEntriesResp)
		}
	}
}

// ---------------------------------------------------------------- helpers

func (s *Server) quorum() int { return len(s.cfg.Peers)/2 + 1 }

func (s *Server) lastLogIndex() uint64 { return uint64(len(s.log) - 1) }

func (s *Server) lastLogTerm() uint64 { return s.log[len(s.log)-1].Term }

func (s *Server) emit(eventType string, index uint64, meta map[string]any) {
	if s.env.Emit != nil {
		s.env.Emit(eventType, s.currentTerm, index, meta)
	}
}

func (s *Server) send(to NodeID, kind MessageKind, fill func(*Message)) {
	m := Message{From: s.cfg.ID, To: to, Kind: kind}
	fill(&m)
	s.env.Send(m)
}

func (s *Server) resetElectionDeadline() {
	d := s.cfg.ElectionTimeoutMin
	if span := s.cfg.ElectionTimeoutMax - s.cfg.ElectionTimeoutMin; span > 0 {
		d += time.Duration(s.rng.Int63n(int64(span) + 1))
	}
	s.electionDeadline = s.env.Now() + d
}

func (s *Server) appendLocal(command string) uint64 {
	index := s.lastLogIndex() + 1
	s.log = append(s.log, LogEntry{Index: index, Term: s.currentTerm, Command: command})
	s.matchIndex[s.cfg.ID] = index
	return index
}

// becomeFollower adopts term if it is newer and steps down from any other role.
func (s *Server) becomeFollower(term uint64) {
	prev := s.role
	if term > s.currentTerm {
		s.currentTerm = term
		s.votedFor = ""
		s.leaderID = ""
	}
	if prev != Follower {
		s.role = Follower
		s.votes = nil
		s.emit("NodeRoleChanged", 0, map[string]any{"from": prev.String(), "to": "follower"})
	}
	if prev == Leader {
		s.resetElectionDeadline()
	}
}

// ---------------------------------------------------------------- election

func (s *Server) startElection() {
	prev := s.role
	s.currentTerm++
	s.role = Candidate
	s.votedFor = s.cfg.ID
	s.leaderID = ""
	s.votes = map[NodeID]bool{s.cfg.ID: true}
	s.resetElectionDeadline()
	s.emit("ElectionStarted", 0, map[string]any{"quorum": s.quorum()})
	s.emit("NodeRoleChanged", 0, map[string]any{"from": prev.String(), "to": "candidate"})

	if len(s.votes) >= s.quorum() { // single-node cluster
		s.becomeLeader()
		return
	}
	req := RequestVoteRequest{
		Term:         s.currentTerm,
		CandidateID:  s.cfg.ID,
		LastLogIndex: s.lastLogIndex(),
		LastLogTerm:  s.lastLogTerm(),
	}
	for _, p := range s.cfg.Peers {
		if p == s.cfg.ID {
			continue
		}
		r := req
		s.send(p, MsgRequestVote, func(m *Message) { m.RequestVote = &r })
	}
}

func (s *Server) candidateUpToDate(req *RequestVoteRequest) bool {
	if req.LastLogTerm != s.lastLogTerm() {
		return req.LastLogTerm > s.lastLogTerm()
	}
	return req.LastLogIndex >= s.lastLogIndex()
}

func (s *Server) handleRequestVote(from NodeID, req *RequestVoteRequest) {
	if req.Term > s.currentTerm {
		s.becomeFollower(req.Term)
	}
	granted := false
	if req.Term == s.currentTerm &&
		(s.votedFor == "" || s.votedFor == req.CandidateID) &&
		s.candidateUpToDate(req) {
		granted = true
		s.votedFor = req.CandidateID
		s.resetElectionDeadline()
	}
	resp := RequestVoteResponse{Term: s.currentTerm, VoteGranted: granted}
	s.send(from, MsgRequestVoteResponse, func(m *Message) { m.RequestVoteResponse = &resp })
	if granted {
		s.emit("VoteGranted", 0, map[string]any{"candidate": string(req.CandidateID)})
	} else {
		s.emit("VoteDenied", 0, map[string]any{"candidate": string(req.CandidateID), "candidate_term": req.Term})
	}
}

func (s *Server) handleRequestVoteResponse(from NodeID, resp *RequestVoteResponse) {
	if resp.Term > s.currentTerm {
		s.becomeFollower(resp.Term)
		return
	}
	if s.role != Candidate || resp.Term != s.currentTerm || !resp.VoteGranted {
		return
	}
	s.votes[from] = true
	if len(s.votes) >= s.quorum() {
		s.becomeLeader()
	}
}

func (s *Server) becomeLeader() {
	s.role = Leader
	s.leaderID = s.cfg.ID
	last := s.lastLogIndex()
	s.nextIndex = make(map[NodeID]uint64, len(s.cfg.Peers))
	s.matchIndex = make(map[NodeID]uint64, len(s.cfg.Peers))
	for _, p := range s.cfg.Peers {
		s.nextIndex[p] = last + 1
	}
	votes := len(s.votes)
	s.votes = nil
	s.emit("NodeRoleChanged", 0, map[string]any{"from": "candidate", "to": "leader"})
	s.emit("LeaderElected", 0, map[string]any{"votes_received": votes, "quorum": s.quorum()})
	// A no-op entry in the new term lets the leader commit earlier-term entries.
	s.appendLocal(NoopCommand)
	s.broadcastAppend()
	s.advanceCommit()
}

// ------------------------------------------------------------- replication

func (s *Server) broadcastAppend() {
	s.heartbeatDeadline = s.env.Now() + s.cfg.HeartbeatInterval
	if s.cfg.VerboseEvents {
		s.emit("HeartbeatSent", 0, nil)
	}
	for _, p := range s.cfg.Peers {
		if p != s.cfg.ID {
			s.sendAppend(p)
		}
	}
}

func (s *Server) sendAppend(peer NodeID) {
	next := s.nextIndex[peer]
	if next < 1 {
		next = 1
	}
	last := s.lastLogIndex()
	if next > last+1 {
		next = last + 1
	}
	prev := next - 1
	var entries []LogEntry
	if next <= last {
		end := last
		if end-next+1 > maxEntriesPerMessage {
			end = next + maxEntriesPerMessage - 1
		}
		entries = append([]LogEntry(nil), s.log[next:end+1]...)
	}
	req := AppendEntriesRequest{
		Term:         s.currentTerm,
		LeaderID:     s.cfg.ID,
		PrevLogIndex: prev,
		PrevLogTerm:  s.log[prev].Term,
		Entries:      entries,
		LeaderCommit: s.commitIndex,
	}
	s.send(peer, MsgAppendEntries, func(m *Message) { m.AppendEntries = &req })
}

func (s *Server) handleAppendEntries(from NodeID, req *AppendEntriesRequest) {
	reply := func(resp AppendEntriesResponse) {
		s.send(from, MsgAppendEntriesResponse, func(m *Message) { m.AppendEntriesResp = &resp })
	}
	if req.Term < s.currentTerm {
		reply(AppendEntriesResponse{Term: s.currentTerm})
		return
	}
	s.becomeFollower(req.Term) // adopts the term, steps down candidates/leaders
	s.leaderID = req.LeaderID
	s.resetElectionDeadline()

	last := s.lastLogIndex()
	if req.PrevLogIndex > last {
		reply(AppendEntriesResponse{Term: s.currentTerm, ConflictIndex: last + 1})
		return
	}
	if s.log[req.PrevLogIndex].Term != req.PrevLogTerm {
		conflictTerm := s.log[req.PrevLogIndex].Term
		conflictIndex := req.PrevLogIndex
		for conflictIndex > 1 && s.log[conflictIndex-1].Term == conflictTerm {
			conflictIndex--
		}
		reply(AppendEntriesResponse{
			Term:          s.currentTerm,
			ConflictIndex: conflictIndex,
			ConflictTerm:  conflictTerm,
		})
		return
	}

	for i, e := range req.Entries {
		idx := req.PrevLogIndex + 1 + uint64(i)
		if idx <= s.lastLogIndex() {
			if s.log[idx].Term == e.Term {
				continue // already present
			}
			if idx <= s.commitIndex {
				// Raft guarantees this cannot happen; refuse and report loudly.
				s.emit("SafetyAnomaly", idx, map[string]any{"reason": "conflict at committed index"})
				reply(AppendEntriesResponse{Term: s.currentTerm, ConflictIndex: idx})
				return
			}
			s.log = s.log[:idx] // delete the conflicting entry and all that follow
		}
		s.log = append(s.log, LogEntry{Index: idx, Term: e.Term, Command: e.Command})
	}
	match := req.PrevLogIndex + uint64(len(req.Entries))

	if req.LeaderCommit > s.commitIndex {
		if nc := min(req.LeaderCommit, match); nc > s.commitIndex {
			s.setCommitIndex(nc)
		}
	}
	if len(req.Entries) > 0 {
		s.emit("AppendEntriesReceived", match, map[string]any{"entries": len(req.Entries), "leader": string(req.LeaderID)})
	}
	reply(AppendEntriesResponse{Term: s.currentTerm, Success: true, MatchIndex: match})
}

func (s *Server) handleAppendEntriesResponse(from NodeID, resp *AppendEntriesResponse) {
	if resp.Term > s.currentTerm {
		s.becomeFollower(resp.Term)
		return
	}
	if s.role != Leader || resp.Term != s.currentTerm {
		return
	}
	if resp.Success {
		if resp.MatchIndex > s.matchIndex[from] {
			s.matchIndex[from] = resp.MatchIndex
			s.emit("LogReplicated", resp.MatchIndex, map[string]any{"follower": string(from)})
		}
		s.nextIndex[from] = s.matchIndex[from] + 1
		s.advanceCommit()
		if s.nextIndex[from] <= s.lastLogIndex() {
			s.sendAppend(from)
		}
		return
	}

	candidate := resp.ConflictIndex
	if resp.ConflictTerm != 0 {
		for i := s.lastLogIndex(); i >= 1; i-- {
			if s.log[i].Term == resp.ConflictTerm {
				candidate = i + 1
				break
			}
			if s.log[i].Term < resp.ConflictTerm {
				break
			}
		}
	}
	if candidate < 1 {
		candidate = 1
	}
	if candidate > s.lastLogIndex()+1 {
		candidate = s.lastLogIndex() + 1
	}
	if floor := s.matchIndex[from] + 1; candidate < floor {
		candidate = floor
	}
	if candidate < s.nextIndex[from] {
		s.nextIndex[from] = candidate
	}
	s.sendAppend(from)
}

// advanceCommit moves commitIndex to the highest index N such that a quorum
// has matchIndex >= N and log[N] belongs to the leader's current term.
func (s *Server) advanceCommit() {
	for n := s.lastLogIndex(); n > s.commitIndex; n-- {
		if s.log[n].Term != s.currentTerm {
			break // terms never decrease along the log
		}
		count := 0
		for _, p := range s.cfg.Peers {
			if s.matchIndex[p] >= n {
				count++
			}
		}
		if count >= s.quorum() {
			s.setCommitIndex(n)
			return
		}
	}
}

func (s *Server) setCommitIndex(n uint64) {
	s.commitIndex = n
	s.emit("EntryCommitted", n, nil)
	for s.lastApplied < s.commitIndex {
		s.lastApplied++
		s.sm.Apply(s.log[s.lastApplied])
	}
}
