# EXP-02

The leader crashes after the cluster has stabilised and never comes back. Measures re-election latency and the time until the surviving quorum is stable again.

All times are **virtual milliseconds** from the deterministic simulator. They are not wall-clock measurements and must not be compared with process-based results.

- Seeds per configuration: 30 (base seed 1)
- Duration per run: 10000 ms
- Election timeout: 150-300 ms, heartbeat: 50 ms
- Fault schedule: crash_leader
- Runs: 90 ok, 0 errors, 0 safety violations in total

## Results (median / mean ± std dev, measured runs of total)

| Nodes | Variant | Metric | Measured | Median | Mean ± SD |
|---|---|---|---|---|---|
| 3 | default | election_latency_ms | 30/30 | 209.75 | 217.79 ± 57.82 |
| 3 | default | reelection_latency_ms | 30/30 | 254.00 | 277.76 ± 110.90 |
| 3 | default | commit_latency_ms_p50 | 30/30 | 24.72 | 24.73 ± 0.25 |
| 3 | default | throughput_commands_per_second | 30/30 | 20.10 | 20.10 ± 0.01 |
| 3 | default | recovery_time_ms | 30/30 | 4013.65 | 4013.64 ± 1.72 |
| 3 | default | message_delivery_rate | 30/30 | 0.79 | 0.79 ± 0.00 |
| 3 | default | failed_elections | 30/30 | 0.00 | 0.60 ± 1.25 |
| 5 | default | election_latency_ms | 30/30 | 195.82 | 202.58 ± 19.35 |
| 5 | default | reelection_latency_ms | 30/30 | 219.43 | 234.91 ± 52.25 |
| 5 | default | commit_latency_ms_p50 | 30/30 | 24.80 | 24.80 ± 0.14 |
| 5 | default | throughput_commands_per_second | 30/30 | 20.10 | 20.10 ± 0.01 |
| 5 | default | recovery_time_ms | 30/30 | 4015.32 | 4015.43 ± 1.39 |
| 5 | default | message_delivery_rate | 30/30 | 0.90 | 0.90 ± 0.00 |
| 5 | default | failed_elections | 30/30 | 0.50 | 0.63 ± 0.76 |
| 7 | default | election_latency_ms | 30/30 | 196.17 | 201.16 ± 17.29 |
| 7 | default | reelection_latency_ms | 30/30 | 213.33 | 221.56 ± 26.52 |
| 7 | default | commit_latency_ms_p50 | 30/30 | 24.89 | 24.85 ± 0.15 |
| 7 | default | throughput_commands_per_second | 30/30 | 20.10 | 20.10 ± 0.00 |
| 7 | default | recovery_time_ms | 30/30 | 4017.13 | 4017.24 ± 2.11 |
| 7 | default | message_delivery_rate | 30/30 | 0.94 | 0.94 ± 0.00 |
| 7 | default | failed_elections | 30/30 | 1.00 | 1.00 ± 1.02 |

Percentiles (P95/P99) and confidence intervals are in `summary.csv`; P95 needs at least 20 samples and P99 at least 100.
