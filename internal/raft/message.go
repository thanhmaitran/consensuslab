package raft

// MessageKind identifies the RPC carried by a Message.
type MessageKind uint8

const (
	MsgRequestVote MessageKind = iota
	MsgRequestVoteResponse
	MsgAppendEntries
	MsgAppendEntriesResponse
)

func (k MessageKind) String() string {
	switch k {
	case MsgRequestVote:
		return "RequestVote"
	case MsgRequestVoteResponse:
		return "RequestVoteResponse"
	case MsgAppendEntries:
		return "AppendEntries"
	case MsgAppendEntriesResponse:
		return "AppendEntriesResponse"
	default:
		return "Unknown"
	}
}

// Message is the envelope exchanged between nodes through the transport.
// Exactly one payload pointer matching Kind is set.
type Message struct {
	ID   uint64
	From NodeID
	To   NodeID
	Kind MessageKind

	RequestVote         *RequestVoteRequest
	RequestVoteResponse *RequestVoteResponse
	AppendEntries       *AppendEntriesRequest
	AppendEntriesResp   *AppendEntriesResponse
}

type RequestVoteRequest struct {
	Term         uint64
	CandidateID  NodeID
	LastLogIndex uint64
	LastLogTerm  uint64
}

type RequestVoteResponse struct {
	Term        uint64
	VoteGranted bool
}

type AppendEntriesRequest struct {
	Term         uint64
	LeaderID     NodeID
	PrevLogIndex uint64
	PrevLogTerm  uint64
	Entries      []LogEntry
	LeaderCommit uint64
}

// AppendEntriesResponse carries the follower's answer. On success MatchIndex
// is the highest index known to match the leader. On failure ConflictIndex and
// ConflictTerm let the leader back up quickly (ConflictTerm == 0 means the
// follower's log was too short).
type AppendEntriesResponse struct {
	Term          uint64
	Success       bool
	MatchIndex    uint64
	ConflictIndex uint64
	ConflictTerm  uint64
}
