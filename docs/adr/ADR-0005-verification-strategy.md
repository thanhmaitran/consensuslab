# ADR-0005 Verification strategy

* Status: Accepted

## Context
A correct-looking final state does not prove that no invariant was violated along the way.

## Decision
An independent verifier observes the node touched by every event, plus pairwise log matching and state-digest checks every 50 events and at the end. Tests combine unit rules, scenario tests INT-01…12 and seeded random fault schedules.

## Alternatives
Check only the final state; rely on unit tests only.

## Consequences
Violations that appear and disappear are still reported. Cost is a log copy per event, acceptable for the cluster sizes (≤ 7). Not a substitute for model checking.
