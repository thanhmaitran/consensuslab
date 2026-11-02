import { useCallback, useEffect, useRef, useState } from "react";
import { api } from "./api/client";
import Controls from "./components/Controls";
import Experiments from "./components/Experiments";
import NodeLogs from "./components/NodeLogs";
import Safety from "./components/Safety";
import Timeline from "./components/Timeline";
import Topology from "./components/Topology";
import type { ClusterResponse, SimEvent } from "./types";

type Page = "overview" | "timeline" | "experiments" | "safety";
const MAX_EVENTS = 3000;

export default function App() {
  const [nodeCount, setNodeCount] = useState(5);
  const [seed, setSeed] = useState(42);
  const [cluster, setCluster] = useState<ClusterResponse | null>(null);
  const [events, setEvents] = useState<SimEvent[]>([]);
  const [page, setPage] = useState<Page>("overview");
  const [selected, setSelected] = useState<string | null>(null);
  const [error, setError] = useState("");
  const lastId = useRef(0);
  const clusterId = cluster?.id;

  const create = useCallback(async () => {
    setError("");
    try {
      if (clusterId) await api.deleteCluster(clusterId).catch(() => undefined);
      lastId.current = 0;
      setEvents([]);
      setSelected(null);
      setCluster(await api.createCluster({ node_count: nodeCount, seed }));
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    }
  }, [clusterId, nodeCount, seed]);

  // Poll the status snapshot. Raft state is only ever read here.
  useEffect(() => {
    if (!clusterId) return;
    const timer = window.setInterval(() => {
      api.getCluster(clusterId).then(setCluster).catch((e: unknown) => setError(e instanceof Error ? e.message : String(e)));
    }, 500);
    return () => window.clearInterval(timer);
  }, [clusterId]);

  // Stream events (SSE). After a reconnect the server resends from `since`; duplicates are dropped by id.
  useEffect(() => {
    if (!clusterId) return;
    const source = new EventSource(api.streamUrl(clusterId, lastId.current));
    const batch: SimEvent[] = [];
    const flush = window.setInterval(() => {
      if (batch.length === 0) return;
      const incoming = batch.splice(0, batch.length);
      setEvents((prev) => [...prev, ...incoming].slice(-MAX_EVENTS));
    }, 250);
    source.addEventListener("sim", (msg) => {
      const ev = JSON.parse((msg as MessageEvent<string>).data) as SimEvent;
      if (ev.event_id <= lastId.current) return;
      lastId.current = ev.event_id;
      batch.push(ev);
    });
    source.onerror = () => setError("Event stream interrupted; the browser will reconnect automatically.");
    source.onopen = () => setError("");
    return () => {
      source.close();
      window.clearInterval(flush);
    };
  }, [clusterId]);

  const status = cluster?.status;
  const pages: [Page, string][] = [
    ["overview", "01 Overview"],
    ["timeline", "02 Event Timeline"],
    ["experiments", "03 Experiments"],
    ["safety", "04 Safety & Verification"],
  ];

  return (
    <div className="app">
      <header>
        <div>
          <h1>ConsensusLab</h1>
          <span className="muted">Distributed systems laboratory · simulation only</span>
        </div>
        <div className="row">
          <label>Nodes
            <select value={nodeCount} onChange={(e) => setNodeCount(Number(e.target.value))}>
              {[3, 4, 5, 6, 7].map((n) => <option key={n}>{n}</option>)}
            </select>
          </label>
          <label>Seed <input type="number" value={seed} onChange={(e) => setSeed(Number(e.target.value))} /></label>
          <button onClick={create}>{cluster ? "Recreate cluster" : "Create cluster"}</button>
        </div>
      </header>

      <nav>
        {pages.map(([id, label]) => (
          <button key={id} className={page === id ? "tab active" : "tab"} onClick={() => setPage(id)}>{label}</button>
        ))}
      </nav>

      {error && <div className="alert" role="alert">{error}</div>}

      {page === "experiments" && <Experiments />}
      {page !== "experiments" && !cluster && <p className="muted">Create a cluster to begin.</p>}

      {cluster && status && page === "overview" && (
        <>
          <div className="stats">
            <div><span>Nodes</span><b>{status.nodeCount}</b></div>
            <div><span>Leader</span><b>{status.leader || "none"}</b></div>
            <div><span>Term</span><b>{status.term}</b></div>
            <div><span>Quorum</span><b>{status.quorum}</b></div>
            <div><span>Committed cmds</span><b>{status.metrics.commandsCommitted}</b></div>
            <div><span>Virtual time</span><b>{(status.nowMs / 1000).toFixed(2)} s</b></div>
            <div><span>Violations</span><b className={status.violations > 0 ? "bad" : ""}>{status.violations}</b></div>
          </div>
          <div className="grid">
            <section className="panel">
              <h3>Cluster topology</h3>
              <Topology nodes={status.nodes} partition={status.partition} selected={selected} onSelect={setSelected} />
              <p className="muted">
                {cluster.running ? "Running" : "Paused"} · seed {status.seed} · trace {status.traceHash.slice(0, 12)}… · dashed red lines are partitioned links. Click a node to select it.
              </p>
            </section>
            <Controls cluster={cluster} onChange={setCluster} onError={setError} onReset={create} selected={selected} />
          </div>
          <section className="panel">
            <h3>Node log replication</h3>
            <NodeLogs nodes={status.nodes} />
          </section>
          <p className="muted">
            Messages sent {status.network.sent}, delivered {status.network.delivered}, lost {status.network.droppedLoss}, dropped by partition {status.network.droppedPartition}.
          </p>
        </>
      )}

      {cluster && page === "timeline" && <Timeline events={events} />}
      {cluster && page === "safety" && <Safety clusterId={cluster.id} tick={status ? status.steps : 0} />}
    </div>
  );
}
