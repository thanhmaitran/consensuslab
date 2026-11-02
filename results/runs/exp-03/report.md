# EXP-03

A 5-node cluster is split 3/2 with the leader placed in the 2-node minority, then healed. The 3-node majority must elect a new leader, the isolated leader must not commit, and logs must converge after healing.

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
| 5 | default | reelection_latency_ms | 30/30 | 195.72 | 228.24 ± 103.53 |
| 5 | default | commit_latency_ms_p50 | 30/30 | 25.07 | 25.05 ± 0.23 |
| 5 | default | throughput_commands_per_second | 30/30 | 12.33 | 12.23 ± 0.17 |
| 5 | default | recovery_time_ms | 30/30 | 25.59 | 25.58 ± 1.65 |
| 5 | default | message_delivery_rate | 30/30 | 0.82 | 0.82 ± 0.00 |
| 5 | default | failed_elections | 30/30 | 0.00 | 0.70 ± 1.02 |

Percentiles (P95/P99) and confidence intervals are in `summary.csv`; P95 needs at least 20 samples and P99 at least 100.
