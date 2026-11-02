#!/usr/bin/env python3
"""Optional analysis of runs.jsonl produced by `consensuslab experiment`.

Standard library only. Prints, per (node_count, variant), the number of runs and
the median of selected metrics, skipping null (unmeasured) values.

Usage: python scripts/analyze.py results/runs/exp-02/runs.jsonl
"""
import json
import statistics
import sys
from collections import defaultdict

METRICS = [
    "election_latency_ms",
    "reelection_latency_ms",
    "commit_latency_ms_p50",
    "throughput_commands_per_second",
    "recovery_time_ms",
    "message_delivery_rate",
]


def main(path):
    groups = defaultdict(list)
    with open(path, encoding="utf-8") as fh:
        for line in fh:
            line = line.strip()
            if line:
                rec = json.loads(line)
                groups[(rec["node_count"], rec["variant"])].append(rec)
    for (nodes, variant), runs in sorted(groups.items()):
        ok = [r for r in runs if r["status"] == "ok"]
        violations = sum(r["safety_violations"] for r in runs)
        print(f"nodes={nodes} variant={variant} runs={len(runs)} ok={len(ok)} violations={violations}")
        for m in METRICS:
            vals = [r[m] for r in ok if r.get(m) is not None]
            if vals:
                print(f"  {m}: n={len(vals)} median={statistics.median(vals):.3f}")
            else:
                print(f"  {m}: not measured")


if __name__ == "__main__":
    if len(sys.argv) != 2:
        sys.exit(__doc__)
    main(sys.argv[1])
