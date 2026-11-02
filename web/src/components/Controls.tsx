import { useState } from "react";
import { api } from "../api/client";
import type { ClusterResponse } from "../types";

interface Props {
  cluster: ClusterResponse;
  onChange: (c: ClusterResponse) => void;
  onError: (m: string) => void;
  onReset: () => void;
  selected: string | null;
}

export default function Controls({ cluster, onChange, onError, onReset, selected }: Props) {
  const [speed, setSpeed] = useState(1);
  const [command, setCommand] = useState("SET name consensuslab");
  const [delay, setDelay] = useState(10);
  const [jitter, setJitter] = useState(5);
  const [loss, setLoss] = useState(0);
  const { id, running, status } = cluster;
  const nodes = status.nodes;

  const run = async (fn: () => Promise<ClusterResponse>) => {
    try {
      onChange(await fn());
    } catch (e) {
      onError(e instanceof Error ? e.message : String(e));
    }
  };

  const isolateLeader = () => {
    if (!status.leader) return onError("No leader to isolate yet");
    const rest = nodes.map((n) => n.id).filter((x) => x !== status.leader);
    void run(() => api.partition(id, [rest, [status.leader]]));
  };
  const splitHalf = () => {
    const ids = nodes.map((n) => n.id);
    const k = Math.ceil(ids.length / 2);
    void run(() => api.partition(id, [ids.slice(0, k), ids.slice(k)]));
  };
  const heal = () => {
    void (async () => {
      try {
        const pid = cluster.partitionId;
        if (!pid) return onError("There is no active partition");
        onChange(await api.heal(id, pid));
      } catch (e) {
        onError(e instanceof Error ? e.message : String(e));
      }
    })();
  };

  return (
    <section className="panel">
      <h3>Simulation controls</h3>
      <div className="row">
        {running ? (
          <button onClick={() => run(() => api.pause(id))}>Pause</button>
        ) : (
          <button onClick={() => run(() => api.start(id, speed))}>Run</button>
        )}
        <button disabled={running} onClick={() => run(() => api.step(id, { events: 1 }))}>Step</button>
        <button disabled={running} onClick={() => run(() => api.step(id, { duration_ms: 100 }))}>+100 ms</button>
        <button onClick={onReset}>Reset</button>
        <label>
          Speed ×
          <select value={speed} onChange={(e) => setSpeed(Number(e.target.value))}>
            {[0.25, 0.5, 1, 2, 5, 10].map((s) => <option key={s} value={s}>{s}</option>)}
          </select>
        </label>
      </div>

      <h4>Faults</h4>
      <div className="row">
        <button disabled={!selected} onClick={() => selected && run(() => api.crash(id, selected))}>
          Crash {selected ?? "(select a node)"}
        </button>
        <button disabled={!selected} onClick={() => selected && run(() => api.restart(id, selected))}>
          Restart {selected ?? ""}
        </button>
      </div>
      <div className="row">
        <button onClick={isolateLeader}>Isolate leader</button>
        <button onClick={splitHalf}>Split in halves</button>
        <button onClick={heal}>Heal partition</button>
      </div>

      <h4>Network</h4>
      <div className="row">
        <label>Delay ms <input type="number" min={0} value={delay} onChange={(e) => setDelay(Number(e.target.value))} /></label>
        <label>Jitter ms <input type="number" min={0} value={jitter} onChange={(e) => setJitter(Number(e.target.value))} /></label>
        <label>Loss <input type="number" min={0} max={1} step={0.05} value={loss} onChange={(e) => setLoss(Number(e.target.value))} /></label>
        <button onClick={() => run(() => api.network(id, { base_delay_ms: delay, jitter_ms: jitter, message_loss_probability: loss }))}>Apply</button>
      </div>

      <h4>Client command</h4>
      <div className="row">
        <input className="grow" value={command} onChange={(e) => setCommand(e.target.value)} aria-label="Command" />
        <button onClick={() => run(() => api.command(id, command))}>Send to leader</button>
      </div>
      <p className="muted">Syntax: <code>SET key value</code> or <code>DELETE key</code>. Commands go to the current leader.</p>
    </section>
  );
}
