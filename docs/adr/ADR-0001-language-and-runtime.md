# ADR-0001 Language and runtime

* Status: Accepted

## Context
The project needs strong support for concurrency primitives, simple deployment and good tooling for testing and race detection.

## Decision
Use Go for the engine, API and experiment runner, standard library only; React + TypeScript for the dashboard; Python optionally for analysis (standard library script provided).

## Alternatives
Python for everything (quick to write, weaker demonstration of concurrency control and interfaces); Rust (steeper learning curve for the goal of the project).

## Consequences
Single static binary, built-in `go test -race`. Frontend and backend are decoupled by HTTP. Go ≥ 1.22 is required (method-aware `ServeMux`, builtin `min`).
