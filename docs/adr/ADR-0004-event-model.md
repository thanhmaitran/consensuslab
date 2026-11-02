# ADR-0004 Event model and streaming

* Status: Accepted

## Context
The dashboard, experiments and replay all need structured history, not log lines.

## Decision
One `Event` schema with virtual time, node, term, index, message id, scenario and seed, no wall-clock time. A bounded recorder keeps recent events and a hash over all events. The API streams events with Server-Sent Events.

## Alternatives
WebSocket (needs a third-party dependency or a hand-written protocol); polling only.

## Consequences
Reproducible traces and a small API. SSE is one-directional, which matches the rule that control goes through REST. Switching to WebSocket later only affects `internal/api` and the client.
