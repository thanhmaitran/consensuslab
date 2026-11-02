# Testing

## Layers

1. **Unit tests** (`internal/*/_test.go`): one rule each — timeout starts an election, one vote per term, stale-log candidates refused, `prevLogIndex` mismatch, conflict truncation, no truncation by delayed duplicates, current-term-only commit (Figure 8), restart semantics, command parsing, network determinism and partitions, statistics, verifier detection of injected violations.
2. **Integration tests** (`tests/integration_test.go`), mapped to the blueprint:

| ID | Scenario | Asserts |
|---|---|---|
| INT-01 | 3 nodes start | exactly one leader |
| INT-02 | command to leader | committed, nodes converge |
| INT-03 | leader crash (5 nodes) | new leader with higher term, commits again |
| INT-04 | follower crash (3 nodes) | still commits |
| INT-05 | follower restart | catches up |
| INT-06 | high latency | no safety violation |
| INT-07 | 20% message loss | no violation, some commits |
| INT-08/10/11 | 3/2 partition, leader in minority, heal | minority cannot commit, majority elects and commits, logs converge, uncommitted minority entry is gone |
| INT-08b | 2/2/1 partition | nothing commits; converges after healing |
| INT-09 | identical election timeouts | contention occurs, never two leaders in a term |
| INT-12 | same seed twice | identical trace hash |

3. **Randomized scenarios** (`tests/random_test.go`): seeded random cluster sizes, delays, loss, crashes, restarts and partitions with a continuous workload. `go test -short` runs 5 seeds, the default 30.
4. **Verifier on every step**: all tests that finish through `mustPass` fail on any invariant violation.

## Reproducing a failure

A failing random test prints its seed and the list of injected inputs. For experiments, a violating run writes `failures/<id>_n<N>_<variant>_seed<S>.json` containing the record, verifier report, inputs and scenario. Reproduce with:

```bash
consensuslab replay -config scenario.json -nodes 5 -variant default -seed 17 -expect <trace_hash>
consensuslab simulate -config scenario.json -nodes 5 -seed 17 -events trace.jsonl
```

## Commands

```bash
go test ./...        # everything
go test -race ./...  # race detector
go vet ./...
```

## Limits of verification

Tests exercise sampled executions; a clean run is evidence, not proof. The race detector only sees executed paths. The verifier checks observed states and the invariants listed in `raft-protocol.md`; it does not model-check all interleavings (see the TLA+ roadmap item).
