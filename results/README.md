# Results

This directory holds **selected** experiment reports and the record schema, not raw output.

- `schema/run-record.schema.json` describes one line of `runs.jsonl`.
- `runs/` holds the outputs of the 450 experiment runs (`runs.jsonl`, `summary.csv`, `report.md`, `scenario.json` per experiment) and is committed as evidence.
- `analysis/` holds tables and figures generated from `runs/` by `scripts/analyze_results.py`, including `data-verification.md`.
- `local-validation/` holds the replay and smoke-run logs from the local validation.
- Large or temporary raw output should go to `results/raw/` or `results/tmp/` (git-ignored).

Status: **measured.** 450 runs in six experiments completed with 0 safety violations; see [../docs/experimental-report.md](../docs/experimental-report.md). Times are virtual simulation time.
