import { useEffect, useRef, useState } from "react";
import { api } from "../api/client";
import type { ExperimentResponse } from "../types";

const DEFAULT_SCENARIO = {
  id: "UI-DEMO",
  description: "Leader crash on a 3-node cluster (small run from the dashboard)",
  node_counts: [3],
  seeds: 5,
  seed_base: 1,
  duration_ms: 6000,
  network: { base_delay_ms: 10, jitter_ms: 5, message_loss_probability: 0 },
  faults: [{ at_ms: 2500, type: "crash_leader" }],
  workload: { commands: 20, start_ms: 1500, interval_ms: 100 },
};

const fmt = (v: number | null | undefined) => (v === null || v === undefined ? "—" : v.toFixed(2));

export default function Experiments() {
  const [text, setText] = useState(JSON.stringify(DEFAULT_SCENARIO, null, 2));
  const [result, setResult] = useState<ExperimentResponse | null>(null);
  const [error, setError] = useState("");
  const timer = useRef<number | undefined>(undefined);

  useEffect(() => () => window.clearInterval(timer.current), []);

  const poll = (id: string) => {
    window.clearInterval(timer.current);
    timer.current = window.setInterval(async () => {
      try {
        const r = await api.getExperiment(id);
        setResult(r);
        if (r.status !== "running") window.clearInterval(timer.current);
      } catch (e) {
        setError(e instanceof Error ? e.message : String(e));
        window.clearInterval(timer.current);
      }
    }, 700);
  };

  const run = async () => {
    setError("");
    setResult(null);
    try {
      const scenario = JSON.parse(text) as object;
      const { id } = await api.runExperiment(scenario);
      poll(id);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    }
  };

  return (
    <section className="panel">
      <h3>Experiments</h3>
      <p className="muted">
        Scenario JSON is the same format as the files in <code>configs/</code>. The API limits seeds, duration and total runs; use the CLI for larger experiments. Times are virtual.
      </p>
      <textarea value={text} onChange={(e) => setText(e.target.value)} rows={16} spellCheck={false} aria-label="Scenario JSON" />
      <div className="row"><button onClick={run}>Run experiment</button></div>
      {error && <p className="bad">{error}</p>}
      {result && (
        <>
          <p>
            {result.id}: <strong>{result.status}</strong> — {result.progress.done}/{result.progress.total} runs · safety violations: {result.safetyViolations}
          </p>
          {result.error && <p className="bad">{result.error}</p>}
          {result.aggregates && (
            <table>
              <thead><tr><th>nodes</th><th>variant</th><th>metric</th><th>measured</th><th>median</th><th>mean ± sd</th><th>P95</th></tr></thead>
              <tbody>
                {result.aggregates.map((a, i) => (
                  <tr key={i}>
                    <td>{a.node_count}</td><td>{a.variant}</td><td>{a.metric}</td>
                    <td>{a.measured}/{a.runs}</td>
                    <td>{a.summary ? fmt(a.summary.median) : "not measured"}</td>
                    <td>{a.summary ? `${fmt(a.summary.mean)} ± ${fmt(a.summary.std_dev)}` : "not measured"}</td>
                    <td>{fmt(a.summary?.p95)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </>
      )}
    </section>
  );
}
