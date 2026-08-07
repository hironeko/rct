export type ActivityStatus =
  | "queued"
  | "running"
  | "waiting"
  | "completed"
  | "failed"
  | "cancelled"
  | "stale";

export interface SafeProgressError {
  code: string;
  summary: string;
  retryable: boolean;
  next_action?: string;
}

export interface CurrentActivity {
  revision: number;
  status: ActivityStatus;
  phase: string;
  action: string;
  role?: string;
  provider?: string;
  backend: string;
  job_id?: string;
  round?: number;
  max_rounds?: number;
  artifact_kind?: string;
  candidate_version?: number;
  previous_verdict?: string;
  required_change_count?: number;
  started_at: string;
  last_heartbeat_at: string;
  waiting_reason?: string;
  error?: SafeProgressError;
}

export interface PhaseProgress {
  id: string;
  label: string;
  status: "not_started" | "running" | "changes_requested" | "approved" | "waiting" | "failed" | "completed";
}

export interface Gauge {
  kind: "macro_phases" | "milestones";
  revision: number;
  completed: number;
  total: number;
  label: string;
  invalidated?: boolean;
  reason?: string;
}

export interface RunSnapshot {
  schema_version: "progress-v1";
  run_id: string;
  project_name: string;
  backend: string;
  mode: "supervised" | "autonomous" | "design-only";
  state: string;
  state_revision: number;
  roles: Record<string, string>;
  activity?: CurrentActivity;
  phases: PhaseProgress[];
  gauges: Gauge[];
  artifacts?: Record<string, string>;
  last_event_seq: number;
  updated_at: string;
  waiting_reason?: string;
  next_action?: string;
}

export interface ProgressEvent {
  schema_version?: string;
  sequence: number;
  timestamp: string;
  run_id: string;
  type: string;
  state_before?: string;
  state_after?: string;
  phase?: string;
  role?: string;
  provider?: string;
  backend?: string;
  job_id?: string;
  round?: number;
  artifact_kind?: string;
  version?: number;
}

export interface ApiEnvelope<T> {
  data?: T;
  error?: { code: string; message: string };
  request_id: string;
}

export interface EventPage {
  events: ProgressEvent[];
  next_after_seq: number;
}
