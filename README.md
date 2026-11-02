# ConsensusLab — Distributed Systems Simulator

[![CI](https://github.com/thanhmaitran/consensuslab/actions/workflows/ci.yml/badge.svg)](https://github.com/thanhmaitran/consensuslab/actions/workflows/ci.yml)

ConsensusLab is an experimental distributed-systems simulator that implements the
[Raft](https://raft.github.io/raft.pdf) consensus protocol in a controlled environment. It models
independent nodes, leader election, replicated logs, network delay, message loss, partitions and node
crashes to investigate consistency, fault tolerance and recovery behaviour.

It combines a deterministic simulation engine, a modular Raft implementation, an independent safety
verifier, reproducible experiments and an interactive dashboard. The goal is to make distributed
consensus **observable, testable and measurable** under controlled fault scenarios.

> ConsensusLab is a simulator for study and research. It is **not** a production consensus service.

## Status

| Area | State |
|---|---|
| Raft core (election, replication, commit, restart) | Implemented (fixed membership, no snapshots, no persistence to disk) |
| Deterministic engine, network faults, partitions | Implemented |
| Safety verifier (5 invariants) | Implemented |
| Unit, integration and randomized tests | Written; run `make test` to execute them |
| Experiment runner (JSON scenarios → JSONL/CSV/Markdown) | Implemented |
| REST + Server-Sent Events API | Implemented |
| React dashboard | Implemented, see `web/` |
| Experimental results | **Pending — not yet measured.** Run `make experiments`; see [docs/experiments.md](docs/experiments.md) |

No performance or correctness claims are made beyond what the tests and your own experiment runs show.
See [docs/limitations.md](docs/limitations.md).

## Features

- 1–7 node clusters that can be started, crashed and restarted; persistent state (term, vote, log) survives a restart, volatile state does not.
- Randomised election timeouts from a per-node seeded RNG, RequestVote, AppendEntries with fast log backtracking, leader no-op, commit only by current-term quorum.
- Virtual time: an ordered event queue drives ticks and message delivery, so a run depends only on configuration, seed and inputs.
- Network simulator: base delay, jitter, loss probability, pairwise-reachability partitions.
- Verifier: election safety, log matching, leader completeness, state-machine safety, index bounds — checked after every event that touches a node.
- Experiment runner with seeded repetitions and honest metrics (`null` means "not measured", never `0`).
- Replay: `consensuslab replay` re-runs a seed and compares trace hashes.
- Dashboard: topology, per-node logs, controls, event timeline, experiments and verification pages.

## Architecture

```
React + TypeScript dashboard  ──REST + SSE──►  Go API server
                                                   │ (serialised access)
                              ┌────────────────────┴────────────────────┐
                              │ Simulation engine (virtual clock, queue) │
                              │   Network ─ delay/loss/partition         │
                              │   Raft nodes (election, replication)     │
                              │   Verifier (invariants, violations)      │
                              └────────────────────┬────────────────────┘
                                     Experiment runner → JSONL / CSV / report.md
```

Details: [docs/architecture.md](docs/architecture.md), [docs/raft-protocol.md](docs/raft-protocol.md),
decisions in [docs/adr/](docs/adr/).

## Quick start

Requirements: Go 1.22+, Node.js 20+ (dashboard only), Git.

```bash
make all                       # gofmt check, vet, tests, build  (Windows: scripts/check.ps1)
bin/consensuslab simulate -config configs/smoke.json -nodes 3 -seed 1
bin/consensuslab replay   -config configs/smoke.json -nodes 3 -seed 1
```

Run the API and the dashboard:

```bash
bin/consensuslab serve -addr :8080          # terminal 1
cd web && npm install && npm run dev        # terminal 2, open http://localhost:5173
```

Or serve the built dashboard from Go: `make web && bin/consensuslab serve -static web/dist`.

Without `make` (PowerShell): `go vet ./...; go test ./...; go build -o bin/consensuslab.exe ./cmd/consensuslab`.

## Configuration

Scenarios are JSON files in `configs/` (unknown fields are rejected):

```json
{
  "id": "EXP-02",
  "node_counts": [3, 5, 7],
  "seeds": 30,
  "duration_ms": 10000,
  "election_timeout_ms": [150, 300],
  "heartbeat_ms": 50,
  "network": { "base_delay_ms": 10, "jitter_ms": 5, "message_loss_probability": 0 },
  "faults": [{ "at_ms": 3000, "type": "crash_leader" }],
  "workload": { "commands": 100, "start_ms": 2000, "interval_ms": 50 }
}
```

Fault types: `crash`, `crash_leader`, `restart`, `partition` (`group_sizes`, `isolate_leader`), `heal`.
Shipped scenarios: `exp-01` baseline, `exp-02` leader crash, `exp-03`/`03b` partitions, `exp-04` network
degradation, `exp-05` restart and recovery, `smoke` for quick checks.

## Experiments

```bash
bin/consensuslab experiment -config configs/exp-02-leader-crash.json -out results/runs/exp-02 -workers 4
```

Each run writes `runs.jsonl` (one record per seed, with trace hash), `summary.csv` (mean, std dev,
median, min, max, P95/P99 when enough samples, 95% CI), `report.md` and, if an invariant failed,
`failures/*.json` with everything needed to replay. Metric definitions: [docs/experiments.md](docs/experiments.md).

## Testing

```bash
go test ./...          # unit + integration (tests/INT-01…INT-12) + randomized scenarios
go test -race ./...    # race detector (the engine is single-threaded; the API is not)
go test -short ./...   # fewer random seeds
```

Strategy and replay of failures: [docs/testing.md](docs/testing.md).

## API

Prefix `/api/v1`: `POST /clusters`, `GET /clusters/{id}`, `POST /clusters/{id}/{start|pause|step}`,
`POST /clusters/{id}/nodes/{nodeId}/{crash|restart}`, `POST|DELETE /clusters/{id}/partitions[/{pid}]`,
`POST /clusters/{id}/commands`, `POST /clusters/{id}/network`, `GET /clusters/{id}/events`,
`GET /clusters/{id}/stream` (SSE), `GET /clusters/{id}/report`, `POST /experiments`, `GET /experiments/{id}`.
Clients never set `currentTerm` or `commitIndex`; they can only issue commands and faults.

## Limitations and roadmap

Fixed membership, in-memory "persistence", no snapshots, no pre-vote or check-quorum, simulated time only.
Roadmap: disk persistence, process-based cluster mode, TLA+ model, snapshotting. See
[docs/limitations.md](docs/limitations.md).

## References

1. D. Ongaro, J. Ousterhout. *In Search of an Understandable Consensus Algorithm.* USENIX ATC 2014. <https://raft.github.io/raft.pdf>
2. Raft project site and TLA+ specification: <https://raft.github.io/>

## License

MIT — see [LICENSE](LICENSE).
