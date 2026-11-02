# ADR-0002 Deterministic simulation with virtual time

* Status: Accepted

## Context
Real timers and goroutine scheduling make distributed tests flaky and unreproducible.

## Decision
Mode A runs all nodes in one goroutine on a virtual clock. An ordered event queue (time, sequence number) drives ticks and message delivery. Each node and the network own seeded RNGs.

## Alternatives
Real timers with goroutines per node (realistic, not reproducible); fully process-based cluster first (harder to debug).

## Consequences
Runs depend only on configuration, seed and inputs; failures replay exactly; the trace hash can prove it. Timing results are in virtual time and are not wall-clock performance.
