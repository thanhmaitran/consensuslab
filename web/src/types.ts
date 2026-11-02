export interface LogEntry {
  index: number;
  term: number;
  command: string;
}

export interface NodeView {
  id: string;
  role: "follower" | "candidate" | "leader";
  currentTerm: number;
  commitIndex: number;
  lastApplied: number;
  logLength: number;
  leaderId: string;
  votedFor: string;
  down: boolean;
  logTail: LogEntry[];
}

export interface ClusterStatus {
  scenarioId: string;
  seed: number;
  nowMs: number;
  steps: number;
  nodeCount: number;
  quorum: number;
  leader: string;
  term: number;
  nodes: NodeView[];
  partition: string[][];
  network: { sent: number; delivered: number; droppedLoss: number; droppedPartition: number; droppedDown: number };
  networkConfig: { baseDelayMs: number; jitterMs: number; lossProbability: number };
  violations: number;
  metrics: {
    commandsSubmitted: number;
    commandsAccepted: number;
    commandsCommitted: number;
    commandsLost: number;
    commandsRejected: number;
    electionsStarted: number;
    leadersElected: number;
  };
  traceHash: string;
}

export interface ClusterResponse {
  id: string;
  running: boolean;
  speed: number;
  status: ClusterStatus;
  partitionId?: string;
}

export interface SimEvent {
  event_id: number;
  virtual_time_ms: number;
  type: string;
  node_id?: string;
  term: number;
  log_index?: number;
  message_id?: number;
  scenario_id?: string;
  seed: number;
  metadata?: Record<string, unknown>;
}

export interface Violation {
  invariant: string;
  node_id?: string;
  term?: number;
  index?: number;
  virtual_time_ms: number;
  detail: string;
}

export interface VerifierReport {
  scenario_id: string;
  seed: number;
  invariants_checked: number;
  observations: number;
  violations: Violation[];
  status: "passed" | "failed";
}

export interface Summary {
  n: number;
  mean: number;
  std_dev: number;
  median: number;
  min: number;
  max: number;
  p95: number | null;
  p99: number | null;
  ci95_half: number | null;
}

export interface Aggregate {
  experiment_id: string;
  node_count: number;
  variant: string;
  metric: string;
  runs: number;
  measured: number;
  summary: Summary | null;
}

export interface ExperimentResponse {
  id: string;
  status: "running" | "done" | "error";
  error: string;
  progress: { done: number; total: number };
  aggregates: Aggregate[] | null;
  safetyViolations: number;
}
