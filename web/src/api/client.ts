import type {
  ClusterResponse,
  ExperimentResponse,
  SimEvent,
  VerifierReport,
} from "../types";

export const API_BASE: string =
  import.meta.env.VITE_API_BASE_URL ?? "http://localhost:8080";

const P = `${API_BASE}/api/v1`;

async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  const response = await fetch(`${P}${path}`, {
    method,
    headers: body === undefined ? undefined : { "Content-Type": "application/json" },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  if (response.status === 204) return undefined as T;
  const data = await response.json().catch(() => ({}));
  if (!response.ok) {
    throw new Error((data as { error?: string }).error ?? `Request failed: ${response.status}`);
  }
  return data as T;
}

const enc = encodeURIComponent;

export const api = {
  createCluster: (body: { node_count: number; seed: number; network?: object }) =>
    request<ClusterResponse>("POST", "/clusters", body),
  getCluster: (id: string) => request<ClusterResponse>("GET", `/clusters/${enc(id)}`),
  deleteCluster: (id: string) => request<void>("DELETE", `/clusters/${enc(id)}`),
  start: (id: string, speed: number) =>
    request<ClusterResponse>("POST", `/clusters/${enc(id)}/start`, { speed }),
  pause: (id: string) => request<ClusterResponse>("POST", `/clusters/${enc(id)}/pause`),
  step: (id: string, body: { events?: number; duration_ms?: number }) =>
    request<ClusterResponse>("POST", `/clusters/${enc(id)}/step`, body),
  crash: (id: string, node: string) =>
    request<ClusterResponse>("POST", `/clusters/${enc(id)}/nodes/${enc(node)}/crash`),
  restart: (id: string, node: string) =>
    request<ClusterResponse>("POST", `/clusters/${enc(id)}/nodes/${enc(node)}/restart`),
  partition: (id: string, groups: string[][]) =>
    request<ClusterResponse>("POST", `/clusters/${enc(id)}/partitions`, { groups }),
  heal: (id: string, partitionId: string) =>
    request<ClusterResponse>("DELETE", `/clusters/${enc(id)}/partitions/${enc(partitionId)}`),
  command: (id: string, command: string, nodeId = "") =>
    request<ClusterResponse>("POST", `/clusters/${enc(id)}/commands`, { command, node_id: nodeId }),
  network: (id: string, body: { base_delay_ms: number; jitter_ms: number; message_loss_probability: number }) =>
    request<ClusterResponse>("POST", `/clusters/${enc(id)}/network`, body),
  events: (id: string, since: number) =>
    request<{ events: SimEvent[]; lastId: number }>("GET", `/clusters/${enc(id)}/events?since=${since}&limit=500`),
  report: (id: string) => request<VerifierReport>("GET", `/clusters/${enc(id)}/report`),
  runExperiment: (scenario: object) =>
    request<{ id: string; total: number }>("POST", "/experiments", { scenario }),
  getExperiment: (id: string) => request<ExperimentResponse>("GET", `/experiments/${enc(id)}`),
  streamUrl: (id: string, since: number) => `${P}/clusters/${enc(id)}/stream?since=${since}`,
};
