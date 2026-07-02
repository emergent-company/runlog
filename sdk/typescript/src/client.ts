import type {
  CreateRunOpts,
  CreateRunResult,
  AddEventOpts,
  MarkDoneOpts,
  DaemonStatus,
  TestRunSummary,
  RunEvent,
  DaemonRun,
} from './types.js'

export class RunLogError extends Error {
  constructor(
    message: string,
    public statusCode?: number,
    public body?: string,
  ) {
    super(message)
    this.name = 'RunLogError'
  }
}

export class RunLogClient {
  constructor(
    private baseUrl: string = 'http://localhost:5002',
    private _fetch: typeof fetch = globalThis.fetch,
  ) {}

  private async request<T>(
    method: string,
    path: string,
    body?: unknown,
  ): Promise<T> {
    const url = `${this.baseUrl}${path}`
    const headers: Record<string, string> = {}
    if (body !== undefined) {
      headers['Content-Type'] = 'application/json'
    }
    const resp = await this._fetch(url, {
      method,
      headers,
      body: body !== undefined ? JSON.stringify(body) : undefined,
    })
    if (!resp.ok) {
      const text = await resp.text().catch(() => '')
      throw new RunLogError(
        `runlog: ${method} ${path} → ${resp.status}`,
        resp.status,
        text,
      )
    }
    if (resp.status === 204) return undefined as T
    return resp.json() as T
  }

  async health(): Promise<boolean> {
    try {
      const resp = await this._fetch(`${this.baseUrl}/health`, { method: 'GET' })
      return resp.ok
    } catch {
      return false
    }
  }

  async status(): Promise<DaemonStatus> {
    return this.request<DaemonStatus>('GET', '/status')
  }

  async createRun(opts: CreateRunOpts): Promise<CreateRunResult> {
    const pid = opts.pid ?? process?.pid ?? 0
    const body: Record<string, unknown> = {
      pid,
      env_profile: opts.envProfile ?? opts.testName,
      description: opts.description,
    }
    if (opts.serverUrl) body.server_url = opts.serverUrl
    if (opts.token) body.token = opts.token
    if (opts.category) body.category = opts.category
    if (opts.tags && opts.tags.length > 0) body.tags = opts.tags
    if (opts.experiment) body.experiment = opts.experiment
    if (opts.runner) body.runner = opts.runner
    if (opts.appVersion) body.app_version = opts.appVersion
    if (opts.testVersion) body.test_version = opts.testVersion
    if (opts.testType) body.test_type = opts.testType
    if (opts.envVars) body.env_vars = opts.envVars
    if (opts.startedAt) body.started_at = opts.startedAt
    if (opts.timeoutSeconds) body.timeout_seconds = opts.timeoutSeconds
    if (opts.coveragePct !== undefined) body.coverage_pct = opts.coveragePct

    return this.request<CreateRunResult>('POST', '/runs', body)
  }

  async markDone(runId: string, opts?: MarkDoneOpts): Promise<void> {
    const body: Record<string, unknown> = {}
    if (opts?.passed !== undefined) body.passed = opts.passed
    if (opts?.skipped !== undefined) body.skipped = opts.skipped
    if (opts?.reason) body.reason = opts.reason
    if (opts?.finishedAt) body.finished_at = opts.finishedAt
    if (opts?.inputTokens !== undefined) body.input_tokens = opts.inputTokens
    if (opts?.outputTokens !== undefined) body.output_tokens = opts.outputTokens
    if (opts?.costUsd !== undefined) body.cost_usd = opts.costUsd
    if (opts?.coveragePct !== undefined) body.coverage_pct = opts.coveragePct

    await this.request<unknown>('PUT', `/runs/${runId}/done`, body)
  }

  async addEvent(runId: string, opts: AddEventOpts): Promise<void> {
    const body: Record<string, unknown> = {
      kind: opts.kind,
      message: opts.message,
      elapsed_s: opts.elapsedS ?? 0,
    }
    if (opts.details) body.details = opts.details

    await this.request<unknown>('POST', `/runs/${runId}/events`, body)
  }

  async section(runId: string, name: string): Promise<void> {
    return this.addEvent(runId, { kind: 'section', message: name })
  }

  async log(runId: string, message: string): Promise<void> {
    return this.addEvent(runId, { kind: 'log', message })
  }

  async fail(runId: string, message: string): Promise<void> {
    return this.addEvent(runId, { kind: 'failure', message })
  }

  async cli(
    runId: string,
    invocation: string,
    output?: string,
  ): Promise<void> {
    const details = output !== undefined ? { output } : undefined
    return this.addEvent(runId, {
      kind: 'cli',
      message: invocation,
      details,
    })
  }

  async setCategory(runId: string, category: string): Promise<void> {
    await this.request<unknown>('PUT', `/runs/${runId}/category`, {
      value: category,
    })
  }

  async setTags(runId: string, tags: string[]): Promise<void> {
    await this.request<unknown>('PUT', `/runs/${runId}/tags`, { tags })
  }

  async setDescription(runId: string, description: string): Promise<void> {
    await this.request<unknown>('PUT', `/runs/${runId}/description`, {
      value: description,
    })
  }

  async setExperiment(runId: string, experiment: string): Promise<void> {
    await this.request<unknown>('PUT', `/runs/${runId}/experiment`, {
      value: experiment,
    })
  }

  async setTestType(runId: string, testType: string): Promise<void> {
    await this.request<unknown>('PUT', `/runs/${runId}/test_type`, {
      value: testType,
    })
  }

  async setVersion(
    runId: string,
    appVersion: string,
    testVersion: string,
  ): Promise<void> {
    await this.request<unknown>('PUT', `/runs/${runId}/version`, {
      app_version: appVersion,
      test_version: testVersion,
    })
  }

  async getRun(runId: string): Promise<DaemonRun> {
    return this.request<DaemonRun>('GET', `/runs/${runId}`)
  }

  async listTestRuns(): Promise<TestRunSummary[]> {
    return this.request<TestRunSummary[]>('GET', '/test-runs')
  }

  async getTestRun(id: number): Promise<TestRunSummary> {
    return this.request<TestRunSummary>('GET', `/test-runs/${id}`)
  }

  async getTestRunEvents(id: number): Promise<RunEvent[]> {
    return this.request<RunEvent[]>('GET', `/test-runs/${id}/events`)
  }
}
