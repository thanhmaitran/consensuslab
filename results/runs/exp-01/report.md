# EXP-01

Baseline clusters of 3, 5 and 7 nodes with no faults and a fixed network delay. Measures election latency and commit latency under reference conditions.

All times are **virtual milliseconds** from the deterministic simulator. They are not wall-clock measurements and must not be compared with process-based results.

- Seeds per configuration: 30 (base seed 1)
- Duration per run: 8000 ms
- Election timeout: 150-300 ms, heartbeat: 50 ms
- Fault schedule: none
- Runs: 90 ok, 0 errors, 0 safety violations in total

## Results (median / mean ± std dev, measured runs of total)

| Nodes | Variant | Metric | Measured | Median | Mean ± SD |
|---|---|---|---|---|---|
| 3 | default | election_latency_ms | 30/30 | 209.75 | 217.79 ± 57.82 |
| 3 | default | reelection_latency_ms | 0/30 | not measured | not measured |
| 3 | default | commit_latency_ms_p50 | 30/30 | 23.81 | 23.80 ± 0.21 |
| 3 | default | throughput_commands_per_second | 30/30 | 20.21 | 20.21 ± 0.01 |
| 3 | default | recovery_time_ms | 0/30 | not measured | not measured |
| 3 | default | message_delivery_rate | 30/30 | 1.00 | 1.00 ± 0.00 |
| 3 | default | failed_elections | 30/30 | 0.00 | 0.33 ± 0.66 |
| 5 | default | election_latency_ms | 30/30 | 195.82 | 202.58 ± 19.35 |
| 5 | default | reelection_latency_ms | 0/30 | not measured | not measured |
| 5 | default | commit_latency_ms_p50 | 30/30 | 24.40 | 24.38 ± 0.18 |
| 5 | default | throughput_commands_per_second | 30/30 | 20.21 | 20.21 ± 0.01 |
| 5 | default | recovery_time_ms | 0/30 | not measured | not measured |
| 5 | default | message_delivery_rate | 30/30 | 1.00 | 1.00 ± 0.00 |
| 5 | default | failed_elections | 30/30 | 0.00 | 0.37 ± 0.56 |
| 7 | default | election_latency_ms | 30/30 | 196.17 | 201.16 ± 17.29 |
| 7 | default | reelection_latency_ms | 0/30 | not measured | not measured |
| 7 | default | commit_latency_ms_p50 | 30/30 | 24.60 | 24.61 ± 0.14 |
| 7 | default | throughput_commands_per_second | 30/30 | 20.21 | 20.21 ± 0.01 |
| 7 | default | recovery_time_ms | 0/30 | not measured | not measured |
| 7 | default | message_delivery_rate | 30/30 | 1.00 | 1.00 ± 0.00 |
| 7 | default | failed_elections | 30/30 | 0.00 | 0.50 ± 0.57 |

Percentiles (P95/P99) and confidence intervals are in `summary.csv`; P95 needs at least 20 samples and P99 at least 100.
