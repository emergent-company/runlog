export interface CreateRunOpts {
  testName: string
  pid?: number
  envProfile?: string
  serverUrl?: string
  token?: string
  category?: string
  tags?: string[]
  description?: string
  experiment?: string
  runner?: string
  appVersion?: string
  testVersion?: string
  testType?: string
  envVars?: Record<string, string>
  startedAt?: string
  timeoutSeconds?: number
  coveragePct?: number
}

export interface CreateRunResult {
  id: string
  testRunId?: number
}

export interface AddEventOpts {
  kind: string
  message: string
  elapsedS?: number
  details?: Record<string, unknown>
}

export interface MarkDoneOpts {
  passed?: boolean
  skipped?: boolean
  reason?: string
  finishedAt?: string
  inputTokens?: number
  outputTokens?: number
  costUsd?: number
  coveragePct?: number
}

export interface DaemonStatus {
  status: string
  pid: number
  port: number
  uptime_s: number
  active_runs: number
  tracked_resources: number
}

export interface TestRunSummary {
  id: number
  test_name: string
  started_at: string
  finished_at?: string
  passed?: number
  skipped: boolean
  category?: string
  runner?: string
  daemon_run_id?: string
}

export interface RunEvent {
  id: number
  seq: number
  occurred_at: string
  elapsed_s: number
  kind: string
  message: string
  details: string
}

export interface DaemonRun {
  id: string
  pid: number
  env_profile: string
  status: string
  started_at: string
  finished_at?: string
  resource_count: number
  resources?: DaemonResource[]
}

export interface DaemonResource {
  id: string
  resource_id: string
  resource_type: string
  status: string
  created_at: string
}
