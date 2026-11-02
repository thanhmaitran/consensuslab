# Raft protocol as implemented

Source: D. Ongaro and J. Ousterhout, *In Search of an Understandable Consensus Algorithm* (<https://raft.github.io/raft.pdf>), Figure 2 and Section 5.

## Conventions

* Log index 0 is a sentinel `(index 0, term 0)`; the first real entry has index 1. `LogLength` = last log index.
* Quorum `Q = floor(N/2) + 1`; the cluster tolerates `F = floor((N-1)/2)` lost nodes **if the rest can still communicate**.
* Membership is fixed. There are no snapshots.

## Election

* A follower or candidate whose election deadline passes increments `currentTerm`, votes for itself, resets its deadline (random in `[min, max]` from its seeded RNG) and sends `RequestVote`.
* A node grants a vote only if the request's term equals its own after adopting any newer term, it has not voted for another candidate in that term, and the candidate's log is at least as up to date (higher last term, or equal last term and index not smaller).
* A candidate becomes leader on a quorum of votes. Any message with a higher term makes a node adopt the term and become follower.
* The election deadline is reset only when the node grants a vote or accepts `AppendEntries` from a current-term leader — not on every message.

## Replication

* On election the leader sets `nextIndex = lastLogIndex+1` for every peer, `matchIndex = 0`, **appends a `NOOP` entry of its term** and broadcasts `AppendEntries`. The no-op lets the leader commit entries from earlier terms indirectly (Section 5.4.2).
* `AppendEntries` is rejected for a stale term or a mismatch at `prevLogIndex`/`prevLogTerm`. Rejections carry `ConflictIndex`/`ConflictTerm` so the leader backs up by term instead of one index per round trip.
* The follower deletes an existing entry and everything after it only when an incoming entry has the same index but a different term. A shorter, delayed message therefore never truncates matching entries. A conflict at an already-committed index is refused and reported as `SafetyAnomaly` (it cannot occur in correct Raft).
* Messages are batched to at most 64 entries.

## Commit and apply

* The leader sets `commitIndex = N` for the highest `N` with `matchIndex >= N` on a quorum **and** `log[N].Term == currentTerm`.
* Followers set `commitIndex = min(leaderCommit, index of last new entry)`.
* `lastApplied` advances to `commitIndex`, applying entries to the key-value state machine (`SET k v`, `DELETE k`, internal `NOOP`).
* Invariant: `0 <= lastApplied <= commitIndex <= lastLogIndex`.

## Restart

Persistent (`currentTerm`, `votedFor`, `log`) survives; role, `commitIndex`, `lastApplied`, leader state and the state machine are reset and rebuilt as the commit index advances again.

## Verified invariants

| Invariant | Check |
|---|---|
| Election safety | at most one leader observed per term |
| Log matching | same (index, term) in two logs ⇒ identical prefix |
| Leader completeness | when a leader of term T is first observed, it holds every entry committed in a term < T |
| State machine safety | the entry committed at an index is identical on every node; equal `lastApplied` ⇒ equal state digest |
| Index bounds | `lastApplied <= commitIndex <= lastLogIndex`, entry index matches position, terms non-decreasing at the tail |

Safety is distinguished from liveness: a cluster without a quorum that fails to commit is behaving correctly.
