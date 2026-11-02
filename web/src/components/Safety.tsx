import { useEffect, useState } from "react";
import { api } from "../api/client";
import type { VerifierReport } from "../types";

export default function Safety({ clusterId, tick }: { clusterId: string; tick: number }) {
  const [report, setReport] = useState<VerifierReport | null>(null);
  const [error, setError] = useState("");

  useEffect(() => {
    let cancelled = false;
    api.report(clusterId)
      .then((r) => { if (!cancelled) { setReport(r); setError(""); } })
      .catch((e: unknown) => { if (!cancelled) setError(e instanceof Error ? e.message : String(e)); });
    return () => { cancelled = true; };
  }, [clusterId, tick]);

  if (error) return <p className="bad">{error}</p>;
  if (!report) return <p className="muted">Loading…</p>;
  const names = ["election_safety", "log_matching", "leader_completeness", "state_machine_safety", "index_bounds"];
  return (
    <section className="panel">
      <h3>Safety &amp; verification</h3>
      <p>
        Status: <span className={report.status === "passed" ? "badge leader" : "badge bad"}>{report.status}</span>{" "}
        · {report.invariants_checked} invariants · {report.observations} observations
      </p>
      <ul>
        {names.map((n) => {
          const count = report.violations.filter((v) => v.invariant === n).length;
          return <li key={n}>{n}: {count === 0 ? "no violation observed" : `${count} violation(s)`}</li>;
        })}
      </ul>
      {report.violations.length > 0 && (
        <table>
          <thead><tr><th>t (ms)</th><th>invariant</th><th>node</th><th>detail</th></tr></thead>
          <tbody>
            {report.violations.map((v, i) => (
              <tr key={i}><td>{v.virtual_time_ms.toFixed(1)}</td><td>{v.invariant}</td><td>{v.node_id}</td><td>{v.detail}</td></tr>
            ))}
          </tbody>
        </table>
      )}
      <p className="muted">A clean report means no violation was observed in this run; it is not a proof of correctness.</p>
    </section>
  );
}
