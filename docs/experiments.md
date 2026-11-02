# Experiments

Time is **virtual**, produced by the deterministic simulator. It must not be compared with wall-clock measurements of a process-based cluster.

## Research questions

* RQ1 Election: effect of election timeout and network delay on election latency.
* RQ2 Fault tolerance: effect of the number of reachable nodes on the ability to commit.
* RQ3 Recovery: time to a stable cluster after a leader crash or after a partition heals.
* RQ4 Throughput: effect of message loss and latency on committed commands per second.
* RQ5 Reproducibility: same seed and configuration ⇒ same trace.

## Variables

Independent: cluster size (3/5/7), base delay, jitter, loss probability, crash/restart schedule, partition shape. Controlled: seed list, workload, election/heartbeat settings, warm-up (workload starts after the first election in shipped scenarios).
Dependent: the metrics below.

## Metric definitions (as implemented)

| Metric | Definition |
|---|---|
| `election_latency_ms` | virtual time of the first `LeaderElected` event (start of the simulation → first leader) |
| `reelection_latency_ms` | first `LeaderElected` after the first scheduled fault, minus that fault's time; `null` if no election followed |
| `commit_latency_ms_*` | per command: commit time − time the leader appended it; P50 always, P95 needs ≥ 20 commands, P99 ≥ 100 |
| `throughput_commands_per_second` | committed commands / (last commit − first accepted command); `null` with < 2 commits |
| `recovery_time_ms` | from fault removal (restart, heal; for a permanent crash the crash itself) until `ClusterStable`: no partition, quorum up, one leader that has committed an entry of its term, all up nodes with identical term, log length and commit index; `null` if not reached |
| `failed_elections` | elections started − distinct terms that produced a leader |
| `message_delivery_rate` | delivered / sent; `null` if nothing was sent |

Commands the client could not place (no leader) are retried every 20 ms; commands accepted by a leader that later lose their entry are counted as `lost` and excluded from latency.

## Scenarios

`configs/exp-01-baseline.json` … `exp-05-restart-recovery.json`; each has a JSON file so it can be rerun without code changes. Shipped defaults use 30 seeds per configuration.

## Reporting

`summary.csv` has runs, measured count, mean, standard deviation, median, min, max, P95/P99 (when enough samples) and the 95% CI half-width (Student t) per metric, node count and variant. Always report configuration file, seed range and code version (`code_version` in each record).

## Results

**Pending — not yet measured.** Run `make experiments` and copy the chosen `report.md` and `summary.csv` here with the commit hash.
