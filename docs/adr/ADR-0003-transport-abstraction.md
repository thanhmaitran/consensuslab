# ADR-0003 Transport abstraction

* Status: Accepted

## Context
Raft nodes must not decide how long a message takes or whether it is lost.

## Decision
Nodes call `Env.Send`; the engine implements `network.Transport` and asks `Network.Route` for delay or drop. Partitions are pairwise reachability, re-checked at delivery time.

## Alternatives
Direct channels between nodes; a global `networkDown` flag.

## Consequences
Faults are injected without touching protocol code, and Mode B can later reuse the same `Node` interface with a real transport.
