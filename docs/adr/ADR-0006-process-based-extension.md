# ADR-0006 Process-based cluster is a later extension

* Status: Accepted

## Context
Real processes add OS scheduling, sockets and harder debugging.

## Decision
Do not build Mode B until Mode A has passing safety tests and a published baseline. When added, it implements the same `raft.Node` surface with a TCP/HTTP transport and is reported separately from virtual-time results.

## Alternatives
Build both modes from the start.

## Consequences
Smaller first release; measurements from the two modes are never mixed.
