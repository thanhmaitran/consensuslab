# Limitations

* Simulated, single-process execution on virtual time; no real sockets, clocks or OS scheduling.
* Fixed membership; no dynamic reconfiguration.
* "Persistence" is in-memory state that survives a simulated restart. There is no disk, no fsync ordering and no torn writes.
* No snapshots or log compaction; logs grow with the workload.
* No pre-vote or check-quorum: a partitioned leader keeps believing it is leader until it hears a higher term, and a rejoining node with a larger term can disturb a stable leader.
* Network model: independent loss, uniform jitter, clean partitions. No duplication, corruption, asymmetric links or Byzantine behaviour.
* Client semantics: commands are not de-duplicated; a command lost by a leader change is counted, not retried. Reads are not linearizable reads.
* The verifier checks the five listed invariants on observed states; it is not a model checker. Leader completeness is checked when a leader is first observed.
* The API is meant for local use and small demos: bounded clusters, steps and experiments, no authentication. Do not expose it publicly without a reverse proxy and access control.
* The dashboard uses Server-Sent Events rather than WebSocket to keep the backend dependency-free.
* Experimental results are virtual-time measurements of this implementation only.
