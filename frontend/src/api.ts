export type JsonValue = string | number | boolean | null | JsonValue[] | { [key: string]: JsonValue }

export interface NodeDefinition {
  id: string
  type: string
  preset?: Record<string, JsonValue>
}

export interface EdgeDefinition {
  from_node: string
  from_branch?: string
  from_port?: string
  to_node: string
  to_input?: string
}

export interface WorkflowDefinition {
  nodes: NodeDefinition[]
  edges: EdgeDefinition[]
}

export interface Workflow {
  workflow_id: string
  uid: number | string
  name: string
  type: string
  definition?: WorkflowDefinition
  created_at?: string
  updated_at?: string
}

export interface Execution {
  execution_id: string
  uid: number | string
  idempotency_key: string
  workflow_id: string
  workflow_type: string
  status: string
  input?: Record<string, JsonValue>
  output?: JsonValue
  internal_error_code?: number
  internal_error_message?: string
  failed_node_id?: string
  created_at?: string
  started_at?: string
  completed_at?: string
  updated_at?: string
}

export interface NodeType {
  type: string
  inputs?: Record<string, { type: string; required: boolean }>
  outputs?: Record<string, { type: string }>
}

export interface Page<T> {
  limit: number
  offset: number
  workflows?: T[]
  executions?: T[]
}

export interface ApiEnvelope<T> {
  data?: T
  error_code?: number
  error_msg?: string
  internal_error_code?: number
  internal_error_msg?: string
}

export interface RequestConfig {
  workflowBase: string
  workerBase: string
}

function withBase(base: string, path: string) {
  if (!base) return path
  return `${base.replace(/\/$/, '')}/${path.replace(/^\//, '')}`
}

export async function request<T>(
  path: string,
  options: RequestInit = {},
  base = '',
): Promise<T> {
  const headers = new Headers(options.headers)
  if (options.body && !headers.has('Content-Type')) headers.set('Content-Type', 'application/json')
  const response = await fetch(withBase(base, path), { ...options, headers })
  const text = await response.text()
  let payload: unknown = null
  try {
    payload = text ? JSON.parse(text) : null
  } catch {
    payload = text
  }
  if (!response.ok) {
    const message = typeof payload === 'object' && payload && 'error_msg' in payload
      ? String((payload as { error_msg?: string }).error_msg || response.statusText)
      : response.statusText
    throw new Error(`${response.status} ${message}`)
  }
  if (typeof payload === 'object' && payload && 'error_code' in payload) {
    const envelope = payload as ApiEnvelope<unknown>
    if (envelope.error_code && envelope.error_code !== 0) {
      throw new Error(envelope.error_msg || `API error ${envelope.error_code}`)
    }
  }
  return payload as T
}

export function listWorkflows(uid: number, config: RequestConfig) {
  return request<ApiEnvelope<Page<Workflow>>>(`/internal/v1/workflows?uid=${uid}&limit=100&offset=0`, {}, config.workflowBase)
}

export function getWorkflow(id: string, config: RequestConfig) {
  return request<ApiEnvelope<Workflow>>(`/internal/v1/workflows/${encodeURIComponent(id)}`, {}, config.workflowBase)
}

export function createWorkflow(body: { uid: number; name: string; type: string; definition: WorkflowDefinition }, config: RequestConfig) {
  return request<ApiEnvelope<Workflow>>('/internal/v1/workflows', {
    method: 'POST',
    body: JSON.stringify(body),
  }, config.workflowBase)
}

export function listExecutions(uid: number, config: RequestConfig) {
  return request<ApiEnvelope<Page<Execution>>>(`/internal/v1/executions?uid=${uid}&limit=100&offset=0`, {}, config.workflowBase)
}

export function getExecution(id: string, config: RequestConfig) {
  return request<ApiEnvelope<Execution>>(`/internal/v1/executions/${encodeURIComponent(id)}`, {}, config.workflowBase)
}

export function createExecution(body: { uid: number; idempotency_key: string; workflow_id: string; input: Record<string, JsonValue> }, config: RequestConfig) {
  return request<ApiEnvelope<{ execution_id: string }>>('/internal/v1/executions', {
    method: 'POST',
    body: JSON.stringify(body),
  }, config.workflowBase)
}

export function listNodeTypes(config: RequestConfig) {
  return request<{ node_types?: NodeType[] }>('/internal/v1/node-types', {}, config.workflowBase)
}

export function health(base: string, path: '/live' | '/ready') {
  return request<unknown>(path, {}, base)
}
