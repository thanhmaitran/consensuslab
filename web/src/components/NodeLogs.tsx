import type { NodeView } from "../types";

export default function NodeLogs({ nodes }: { nodes: NodeView[] }) {
  return (
    <div className="logs">
      {nodes.map((n) => (
        <div key={n.id} className="log-row">
          <div className="log-head">
            <strong>{n.id}</strong>
            <span className={`badge ${n.down ? "down" : n.role}`}>{n.down ? "down" : n.role}</span>
            <span className="muted">
              committed {n.commitIndex}/{n.logLength} · applied {n.lastApplied}
            </span>
          </div>
          <div className="cells">
            {n.logTail.length === 0 && <span className="muted">empty log</span>}
            {n.logTail.map((e) => (
              <span
                key={e.index}
                className={e.index <= n.commitIndex ? "cell committed" : "cell"}
                title={`index ${e.index}, term ${e.term}: ${e.command}`}
              >
                <small>{e.index}</small>
                <b>t{e.term}</b>
              </span>
            ))}
          </div>
        </div>
      ))}
      <p className="muted">Filled cells are committed. Hover a cell for index, term and command. Last 32 entries shown.</p>
    </div>
  );
}
