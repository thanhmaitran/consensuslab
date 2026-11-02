import type { NodeView } from "../types";

interface Props {
  nodes: NodeView[];
  partition: string[][];
  selected: string | null;
  onSelect: (id: string) => void;
}

const ROLE_COLOR: Record<string, string> = {
  leader: "var(--leader)",
  candidate: "var(--candidate)",
  follower: "var(--follower)",
};

function groupOf(id: string, partition: string[][]): number {
  const i = partition.findIndex((g) => g.includes(id));
  return i < 0 ? 0 : i + 1;
}

export default function Topology({ nodes, partition, selected, onSelect }: Props) {
  const size = 360;
  const c = size / 2;
  const r = 130;
  const pos = nodes.map((n, i) => {
    const a = (2 * Math.PI * i) / Math.max(nodes.length, 1) - Math.PI / 2;
    return { id: n.id, x: c + r * Math.cos(a), y: c + r * Math.sin(a) };
  });
  const byId = new Map(pos.map((p) => [p.id, p]));

  return (
    <svg viewBox={`0 0 ${size} ${size}`} className="topology" role="img" aria-label="Cluster topology">
      {nodes.flatMap((a, i) =>
        nodes.slice(i + 1).map((b) => {
          const pa = byId.get(a.id)!;
          const pb = byId.get(b.id)!;
          const cut = groupOf(a.id, partition) !== groupOf(b.id, partition);
          return (
            <line
              key={`${a.id}-${b.id}`}
              x1={pa.x}
              y1={pa.y}
              x2={pb.x}
              y2={pb.y}
              className={cut ? "edge cut" : "edge"}
            />
          );
        }),
      )}
      {nodes.map((n) => {
        const p = byId.get(n.id)!;
        const label = n.down ? "DOWN" : n.role.toUpperCase();
        return (
          <g key={n.id} onClick={() => onSelect(n.id)} className="node" tabIndex={0} role="button"
             onKeyDown={(e) => e.key === "Enter" && onSelect(n.id)}>
            <circle
              cx={p.x}
              cy={p.y}
              r={32}
              fill={n.down ? "var(--surface)" : ROLE_COLOR[n.role]}
              stroke={selected === n.id ? "var(--text)" : "var(--border)"}
              strokeWidth={selected === n.id ? 3 : 1.5}
              strokeDasharray={n.down ? "5 4" : undefined}
              opacity={n.down ? 0.6 : 1}
            />
            <text x={p.x} y={p.y - 6} textAnchor="middle" className="node-name">{n.id.replace("node-", "N")}</text>
            <text x={p.x} y={p.y + 8} textAnchor="middle" className="node-role">{label}</text>
            <text x={p.x} y={p.y + 21} textAnchor="middle" className="node-term">term {n.currentTerm}</text>
          </g>
        );
      })}
    </svg>
  );
}
