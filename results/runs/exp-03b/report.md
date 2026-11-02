# EXP-03B

A 5-node cluster is split 2/2/1 so that no group has a quorum, then healed. Commands cannot commit during the partition; safety must still hold.

All times are **virtual milliseconds** from the deterministic simulator. They are not wall-clock measurements and must not be compared with process-based results.

- Seeds per configuration: 30 (base seed 1)
- Duration per run: 12000 ms
- Election timeout: 150-300 ms, heartbeat: 50 ms
- Fault schedule: partition+heal
- Runs: 30 ok, 0 errors, 0 safety violations in total

## Results (median / mean ± std dev, measured runs of total)

| Nodes | Variant | Metric | Measured | Median | Mean ± SD |
|---|---|---|---|---|---|
| 5 | default | election_latency_ms | 30/30 | 195.82 | 202.58 ± 19.35 |
| 5 | default | reelection_latency_ms | 30/30 | 4151.86 | 4139.77 ± 78.25 |
| 5 | default | commit_latency_ms_p50 | 30/30 | 24.41 | 50.49 ± 60.04 |
| 5 | default | throughput_commands_per_second | 30/30 | 6.29 | 7.34 ± 2.39 |
| 5 | default | recovery_time_ms | 30/30 | 264.09 | 254.85 ± 108.39 |
| 5 | default | message_delivery_rate | 30/30 | 0.80 | 0.79 ± 0.02 |
| 5 | default | failed_elections | 30/30 | 40.00 | 40.57 ± 2.50 |

Percentiles (P95/P99) and confidence intervals are in `summary.csv`; P95 needs at least 20 samples and P99 at least 100.
