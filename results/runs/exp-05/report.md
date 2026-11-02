# EXP-05

A follower crashes, the cluster keeps committing, then the follower restarts and must catch up. Measures time until every node has the same log and commit index.

All times are **virtual milliseconds** from the deterministic simulator. They are not wall-clock measurements and must not be compared with process-based results.

- Seeds per configuration: 30 (base seed 1)
- Duration per run: 12000 ms
- Election timeout: 150-300 ms, heartbeat: 50 ms
- Fault schedule: crash
- Runs: 60 ok, 0 errors, 0 safety violations in total

## Results (median / mean ± std dev, measured runs of total)

| Nodes | Variant | Metric | Measured | Median | Mean ± SD |
|---|---|---|---|---|---|
| 3 | default | election_latency_ms | 30/30 | 209.75 | 217.79 ± 57.82 |
| 3 | default | reelection_latency_ms | 10/30 | 216.13 | 292.65 ± 180.64 |
| 3 | default | commit_latency_ms_p50 | 30/30 | 24.50 | 24.47 ± 0.30 |
| 3 | default | throughput_commands_per_second | 30/30 | 16.77 | 16.77 ± 0.00 |
| 3 | default | recovery_time_ms | 30/30 | 1004.25 | 1004.00 ± 1.44 |
| 3 | default | message_delivery_rate | 30/30 | 0.90 | 0.90 ± 0.00 |
| 3 | default | failed_elections | 30/30 | 0.00 | 0.53 ± 0.97 |
| 5 | default | election_latency_ms | 30/30 | 195.82 | 202.58 ± 19.35 |
| 5 | default | reelection_latency_ms | 7/30 | 209.68 | 249.37 ± 111.97 |
| 5 | default | commit_latency_ms_p50 | 30/30 | 24.81 | 24.80 ± 0.17 |
| 5 | default | throughput_commands_per_second | 30/30 | 16.77 | 16.77 ± 0.00 |
| 5 | default | recovery_time_ms | 30/30 | 1005.23 | 1005.62 ± 1.75 |
| 5 | default | message_delivery_rate | 30/30 | 0.95 | 0.95 ± 0.00 |
| 5 | default | failed_elections | 30/30 | 0.00 | 0.57 ± 0.86 |

Percentiles (P95/P99) and confidence intervals are in `summary.csv`; P95 needs at least 20 samples and P99 at least 100.
