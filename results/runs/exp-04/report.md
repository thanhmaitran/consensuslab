# EXP-04

Increasing delay, jitter and message loss on a 5-node cluster without crashes.

All times are **virtual milliseconds** from the deterministic simulator. They are not wall-clock measurements and must not be compared with process-based results.

- Seeds per configuration: 30 (base seed 1)
- Duration per run: 15000 ms
- Election timeout: 150-300 ms, heartbeat: 50 ms
- Fault schedule: none
- Runs: 150 ok, 0 errors, 0 safety violations in total

## Results (median / mean ± std dev, measured runs of total)

| Nodes | Variant | Metric | Measured | Median | Mean ± SD |
|---|---|---|---|---|---|
| 5 | delay10-loss0 | election_latency_ms | 30/30 | 195.82 | 202.58 ± 19.35 |
| 5 | delay10-loss0 | reelection_latency_ms | 0/30 | not measured | not measured |
| 5 | delay10-loss0 | commit_latency_ms_p50 | 30/30 | 24.37 | 24.36 ± 0.16 |
| 5 | delay10-loss0 | throughput_commands_per_second | 30/30 | 20.11 | 20.11 ± 0.00 |
| 5 | delay10-loss0 | recovery_time_ms | 0/30 | not measured | not measured |
| 5 | delay10-loss0 | message_delivery_rate | 30/30 | 1.00 | 1.00 ± 0.00 |
| 5 | delay10-loss0 | failed_elections | 30/30 | 0.00 | 0.37 ± 0.56 |
| 5 | delay30-loss0 | election_latency_ms | 30/30 | 247.91 | 271.42 ± 77.95 |
| 5 | delay30-loss0 | reelection_latency_ms | 0/30 | not measured | not measured |
| 5 | delay30-loss0 | commit_latency_ms_p50 | 30/30 | 66.19 | 66.21 ± 0.17 |
| 5 | delay30-loss0 | throughput_commands_per_second | 30/30 | 19.94 | 19.94 ± 0.01 |
| 5 | delay30-loss0 | recovery_time_ms | 0/30 | not measured | not measured |
| 5 | delay30-loss0 | message_delivery_rate | 30/30 | 1.00 | 1.00 ± 0.00 |
| 5 | delay30-loss0 | failed_elections | 30/30 | 1.00 | 1.27 ± 1.60 |
| 5 | delay10-loss5 | election_latency_ms | 30/30 | 197.38 | 203.21 ± 19.00 |
| 5 | delay10-loss5 | reelection_latency_ms | 0/30 | not measured | not measured |
| 5 | delay10-loss5 | commit_latency_ms_p50 | 30/30 | 24.64 | 24.64 ± 0.16 |
| 5 | delay10-loss5 | throughput_commands_per_second | 30/30 | 20.11 | 20.10 ± 0.01 |
| 5 | delay10-loss5 | recovery_time_ms | 0/30 | not measured | not measured |
| 5 | delay10-loss5 | message_delivery_rate | 30/30 | 0.95 | 0.95 ± 0.00 |
| 5 | delay10-loss5 | failed_elections | 30/30 | 0.00 | 0.40 ± 0.62 |
| 5 | delay10-loss20 | election_latency_ms | 30/30 | 216.34 | 251.53 ± 104.43 |
| 5 | delay10-loss20 | reelection_latency_ms | 0/30 | not measured | not measured |
| 5 | delay10-loss20 | commit_latency_ms_p50 | 30/30 | 25.54 | 25.55 ± 0.26 |
| 5 | delay10-loss20 | throughput_commands_per_second | 30/30 | 20.09 | 20.02 ± 0.21 |
| 5 | delay10-loss20 | recovery_time_ms | 0/30 | not measured | not measured |
| 5 | delay10-loss20 | message_delivery_rate | 30/30 | 0.80 | 0.80 ± 0.01 |
| 5 | delay10-loss20 | failed_elections | 30/30 | 3.00 | 3.90 ± 3.35 |
| 5 | delay60-loss10 | election_latency_ms | 30/30 | 498.22 | 502.39 ± 154.76 |
| 5 | delay60-loss10 | reelection_latency_ms | 0/30 | not measured | not measured |
| 5 | delay60-loss10 | commit_latency_ms_p50 | 30/30 | 137.42 | 137.39 ± 0.62 |
| 5 | delay60-loss10 | throughput_commands_per_second | 30/30 | 19.66 | 19.65 ± 0.02 |
| 5 | delay60-loss10 | recovery_time_ms | 0/30 | not measured | not measured |
| 5 | delay60-loss10 | message_delivery_rate | 30/30 | 0.90 | 0.90 ± 0.00 |
| 5 | delay60-loss10 | failed_elections | 30/30 | 5.50 | 7.23 ± 4.90 |

Percentiles (P95/P99) and confidence intervals are in `summary.csv`; P95 needs at least 20 samples and P99 at least 100.
