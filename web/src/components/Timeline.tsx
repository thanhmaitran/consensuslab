import { useMemo, useState } from "react";
import type { SimEvent } from "../types";

const MAX_ROWS = 300;

export default function Timeline({ events }: { events: SimEvent[] }) {
  const [node, setNode] = useState("");
  const [type, setType] = useState("");
  const [query, setQuery] = useState("");
  const [picked, setPicked] = useState<SimEvent | null>(null);

  const types = useMemo(() => Array.from(new Set(events.map((e) => e.type))).sort(), [events]);
  const nodes = useMemo(() => Array.from(new Set(events.map((e) => e.node_id ?? "").filter(Boolean))).sort(), [events]);

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase();
    return events.filter((e) =>
      (!node || e.node_id === node) &&
      (!type || e.type === type) &&
      (!q || JSON.stringify(e).toLowerCase().includes(q)),
    );
  }, [events, node, type, query]);
  const rows = filtered.slice(-MAX_ROWS).reverse();

  return (
    <section className="panel">
      <h3>Event timeline <span className="muted">(virtual time)</span></h3>
      <div className="row">
        <select value={node} onChange={(e) => setNode(e.target.value)} aria-label="Node filter">
          <option value="">all nodes</option>
          {nodes.map((n) => <option key={n}>{n}</option>)}
        </select>
        <select value={type} onChange={(e) => setType(e.target.value)} aria-label="Type filter">
          <option value="">all types</option>
          {types.map((t) => <option key={t}>{t}</option>)}
        </select>
        <input className="grow" placeholder="search…" value={query} onChange={(e) => setQuery(e.target.value)} />
      </div>
      <p className="muted">{filtered.length} matching of {events.length} received; showing the latest {Math.min(MAX_ROWS, filtered.length)}.</p>
      <div className="timeline">
        <table>
          <thead><tr><th>#</th><th>t (ms)</th><th>node</th><th>type</th><th>term</th><th>idx</th></tr></thead>
          <tbody>
            {rows.map((e) => (
              <tr key={e.event_id} onClick={() => setPicked(e)} className={picked?.event_id === e.event_id ? "picked" : ""}>
                <td>{e.event_id}</td>
                <td>{e.virtual_time_ms.toFixed(1)}</td>
                <td>{e.node_id ?? ""}</td>
                <td className={e.type === "SafetyViolationDetected" ? "bad" : ""}>{e.type}</td>
                <td>{e.term}</td>
                <td>{e.log_index ?? ""}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {picked && <pre className="detail">{JSON.stringify(picked, null, 2)}</pre>}
    </section>
  );
}
