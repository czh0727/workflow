<script setup lang="ts">
import { computed, onMounted, ref, shallowRef } from 'vue'
import {
  createExecution,
  createWorkflow,
  getExecution,
  getWorkflow,
  health,
  listExecutions,
  listNodeTypes,
  listWorkflows,
  request,
  type Execution,
  type JsonValue,
  type NodeType,
  type RequestConfig,
  type Workflow,
  type WorkflowDefinition,
} from './api'

type View = 'overview' | 'workflows' | 'executions' | 'temporal' | 'nodes' | 'playground'
type HealthState = 'checking' | 'up' | 'down'

const activeView = ref<View>('overview')
const uid = ref(Number(localStorage.getItem('workflow.uid') || '1'))
const workflowBase = ref(localStorage.getItem('workflow.workflowBase') || import.meta.env.VITE_WORKFLOW_API_BASE || '')
const workerBase = ref(localStorage.getItem('workflow.workerBase') || import.meta.env.VITE_WORKER_API_BASE || '/worker')
const temporalDefaultUrl = import.meta.env.VITE_TEMPORAL_UI_URL || 'http://127.0.0.1:18082/namespaces/default/workflows'
const savedTemporalUrl = localStorage.getItem('workflow.temporalUrl')
const temporalUrl = ref(savedTemporalUrl === 'http://localhost:18082' ? temporalDefaultUrl : (savedTemporalUrl || temporalDefaultUrl))
const workflows = shallowRef<Workflow[]>([])
const executions = shallowRef<Execution[]>([])
const nodeTypes = shallowRef<NodeType[]>([])
const selectedWorkflow = shallowRef<Workflow | null>(null)
const selectedExecution = shallowRef<Execution | null>(null)
const loading = ref(false)
const refreshing = ref(false)
const createOpen = ref(false)
const runOpen = ref(false)
const settingsOpen = ref(false)
const detailTab = ref<'input' | 'output' | 'timeline'>('input')
const executionFilter = ref('all')
const workflowQuery = ref('')
const toast = ref('')
const toastKind = ref<'success' | 'error' | 'info'>('info')
const apiHealth = ref<HealthState>('checking')
const workerHealth = ref<HealthState>('checking')
const temporalHealth = ref<HealthState>('checking')
const apiResponse = ref('')
const playgroundBusy = ref(false)
const playgroundMethod = ref('GET')
const playgroundPath = ref('/internal/v1/node-types')
const playgroundBody = ref('')

const newWorkflowName = ref('')
const newWorkflowType = ref('template')
const newWorkflowDefinition = ref(JSON.stringify({
  nodes: [
    { id: 'say', type: 'SAY_SOMETHING', preset: { prompt: 'hello from the control room' } },
    { id: 'output', type: 'OUTPUT', preset: {} },
  ],
  edges: [
    { from_node: 'say', from_port: 'text', to_node: 'output', to_input: 'text' },
  ],
}, null, 2))
const runInput = ref('{}')

const config = computed<RequestConfig>(() => ({ workflowBase: workflowBase.value, workerBase: workerBase.value }))
const pageTitle = computed(() => ({
  overview: 'Overview',
  workflows: 'Workflows',
  executions: 'Executions',
  temporal: 'Temporal UI',
  nodes: 'Node catalog',
  playground: 'API playground',
}[activeView.value]))
const activeStatusSet = new Set<string>(['running', 'pending', 'started', 'queued'])
const failedStatusSet = new Set<string>(['failed', 'error', 'cancelled', 'canceled', 'timed_out'])
const completedStatusSet = new Set<string>(['completed', 'success', 'succeeded'])
const activeExecutions = computed<number>(() => executions.value.reduce((count, item) => count + (activeStatusSet.has(item.status.toLowerCase()) ? 1 : 0), 0))
const failedExecutions = computed<number>(() => executions.value.reduce((count, item) => count + (failedStatusSet.has(item.status.toLowerCase()) ? 1 : 0), 0))
const completedExecutions = computed<number>(() => executions.value.reduce((count, item) => count + (completedStatusSet.has(item.status.toLowerCase()) ? 1 : 0), 0))
const filteredWorkflows = computed(() => workflows.value.filter((item) => `${item.name} ${item.type} ${item.workflow_id}`.toLowerCase().includes(workflowQuery.value.toLowerCase())))
const filteredExecutions = computed(() => executionFilter.value === 'all' ? executions.value : executions.value.filter((item) => item.status.toLowerCase() === executionFilter.value))
const recentExecutions = computed(() => [...executions.value].sort((a, b) => dateValue(b.updated_at || b.created_at) - dateValue(a.updated_at || a.created_at)).slice(0, 6))

const navItems: { key: View; label: string; icon: string; hint: string }[] = [
  { key: 'overview', label: 'Overview', icon: '⌂', hint: '运行概览' },
  { key: 'workflows', label: 'Workflows', icon: '◇', hint: '定义与触发' },
  { key: 'executions', label: 'Executions', icon: '↗', hint: '执行与排障' },
  { key: 'temporal', label: 'Temporal UI', icon: '◒', hint: '原生工作流视图' },
  { key: 'nodes', label: 'Node catalog', icon: '▦', hint: '节点与端口' },
  { key: 'playground', label: 'API playground', icon: '⌘', hint: 'HTTP 调试' },
]

function notify(message: string, kind: 'success' | 'error' | 'info' = 'info') {
  toast.value = message
  toastKind.value = kind
  window.setTimeout(() => { if (toast.value === message) toast.value = '' }, 4200)
}

function dateValue(value?: string) {
  if (!value) return 0
  const parsed = Date.parse(value)
  return Number.isNaN(parsed) ? 0 : parsed
}

function formatDate(value?: string) {
  if (!value) return '—'
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? value : new Intl.DateTimeFormat('zh-CN', { month: 'short', day: '2-digit', hour: '2-digit', minute: '2-digit' }).format(date)
}

function shortId(value?: string, size = 14) {
  if (!value) return '—'
  return value.length > size ? `${value.slice(0, size)}…` : value
}

function statusClass(status?: string) {
  const normalized = (status || 'unknown').toLowerCase()
  if (['completed', 'success', 'succeeded', 'up'].includes(normalized)) return 'success'
  if (['failed', 'error', 'cancelled', 'canceled', 'timed_out', 'down'].includes(normalized)) return 'danger'
  if (['running', 'pending', 'started', 'queued', 'checking'].includes(normalized)) return 'warning'
  return 'neutral'
}

function statusLabel(status?: string) {
  const normalized = (status || 'unknown').toLowerCase()
  const labels: Record<string, string> = { completed: 'Completed', success: 'Success', succeeded: 'Succeeded', failed: 'Failed', error: 'Error', cancelled: 'Cancelled', canceled: 'Cancelled', running: 'Running', pending: 'Pending', started: 'Started', queued: 'Queued', timed_out: 'Timed out', up: 'Online', down: 'Offline', checking: 'Checking' }
  return labels[normalized] || status || 'Unknown'
}

function valueText(value: unknown) {
  if (value === undefined || value === null) return '—'
  return typeof value === 'string' ? value : JSON.stringify(value)
}

function jsonPretty(value: unknown) {
  if (value === undefined || value === null) return '{}'
  try { return JSON.stringify(value, null, 2) } catch { return String(value) }
}

async function refreshAll(silent = false) {
  if (refreshing.value) return
  refreshing.value = true
  loading.value = true
  const [workflowResult, executionResult, nodeResult, apiResult, workerResult] = await Promise.allSettled([
    listWorkflows(uid.value, config.value),
    listExecutions(uid.value, config.value),
    listNodeTypes(config.value),
    health(workflowBase.value, '/ready'),
    health(workerBase.value, '/ready'),
  ])
  if (workflowResult.status === 'fulfilled') workflows.value = workflowResult.value.data?.workflows || []
  if (executionResult.status === 'fulfilled') executions.value = executionResult.value.data?.executions || []
  if (nodeResult.status === 'fulfilled') nodeTypes.value = nodeResult.value.node_types || []
  apiHealth.value = apiResult.status === 'fulfilled' ? 'up' : 'down'
  workerHealth.value = workerResult.status === 'fulfilled' ? 'up' : 'down'
  temporalHealth.value = 'checking'
  loading.value = false
  refreshing.value = false
  if (!silent) {
    const errors = [workflowResult, executionResult, nodeResult].filter((item) => item.status === 'rejected')
    notify(errors.length ? `已刷新，但有 ${errors.length} 个接口不可用` : '数据已刷新', errors.length ? 'error' : 'success')
  }
}

function openCreate() {
  createOpen.value = true
  newWorkflowName.value = `学习工作流 ${workflows.value.length + 1}`
}

function openRun(workflow: Workflow) {
  selectedWorkflow.value = workflow
  runInput.value = '{}'
  runOpen.value = true
}

function runSelectedWorkflow() {
  if (selectedWorkflow.value) openRun(selectedWorkflow.value)
}

async function submitWorkflow() {
  try {
    const definition = JSON.parse(newWorkflowDefinition.value) as WorkflowDefinition
    if (!Array.isArray(definition.nodes) || !Array.isArray(definition.edges)) throw new Error('definition 需要包含 nodes 和 edges')
    await createWorkflow({ uid: uid.value, name: newWorkflowName.value.trim(), type: newWorkflowType.value.trim(), definition }, config.value)
    createOpen.value = false
    await refreshAll(true)
    notify('工作流已创建', 'success')
    activeView.value = 'workflows'
  } catch (error) {
    notify(error instanceof Error ? error.message : '创建工作流失败', 'error')
  }
}

async function submitRun() {
  if (!selectedWorkflow.value) return
  try {
    const input = JSON.parse(runInput.value) as Record<string, JsonValue>
    const response = await createExecution({ uid: uid.value, idempotency_key: `console-${Date.now()}`, workflow_id: selectedWorkflow.value.workflow_id, input }, config.value)
    runOpen.value = false
    await refreshAll(true)
    notify(`执行已创建：${shortId(response.data?.execution_id)}`, 'success')
    activeView.value = 'executions'
  } catch (error) {
    notify(error instanceof Error ? error.message : '启动执行失败', 'error')
  }
}

async function showWorkflow(workflow: Workflow) {
  try {
    const response = await getWorkflow(workflow.workflow_id, config.value)
    selectedWorkflow.value = response.data || workflow
  } catch {
    selectedWorkflow.value = workflow
  }
}

async function showExecution(execution: Execution) {
  selectedExecution.value = execution
  detailTab.value = 'input'
  try {
    const response = await getExecution(execution.execution_id, config.value)
    selectedExecution.value = response.data || execution
  } catch (error) {
    notify(error instanceof Error ? error.message : '获取执行详情失败', 'error')
  }
}

function persistSettings() {
  localStorage.setItem('workflow.uid', String(uid.value))
  localStorage.setItem('workflow.workflowBase', workflowBase.value)
  localStorage.setItem('workflow.workerBase', workerBase.value)
  localStorage.setItem('workflow.temporalUrl', temporalUrl.value)
  settingsOpen.value = false
  refreshAll(true)
  notify('连接设置已保存', 'success')
}

async function runPlayground() {
  playgroundBusy.value = true
  apiResponse.value = ''
  try {
    const headers: Record<string, string> = {}
    const options: RequestInit = { method: playgroundMethod.value, headers }
    if (playgroundMethod.value !== 'GET' && playgroundBody.value.trim()) {
      headers['Content-Type'] = 'application/json'
      options.body = playgroundBody.value
    }
    const response = await request<unknown>(playgroundPath.value, options, workflowBase.value)
    apiResponse.value = jsonPretty(response)
  } catch (error) {
    apiResponse.value = JSON.stringify({ error: error instanceof Error ? error.message : String(error) }, null, 2)
  } finally {
    playgroundBusy.value = false
  }
}

function usePlaygroundExample(path: string, method = 'GET', body = '') {
  playgroundPath.value = path
  playgroundMethod.value = method
  playgroundBody.value = body
  activeView.value = 'playground'
}

onMounted(() => refreshAll(true))
</script>

<template>
  <div class="app-shell">
    <aside class="sidebar">
      <div class="brand">
        <div class="brand-mark"><span></span><span></span><span></span></div>
        <div><strong>Flowroom</strong><small>Temporal lab</small></div>
      </div>
      <div class="workspace-switcher">
        <div class="workspace-avatar">L</div>
        <div class="workspace-copy"><strong>Local workspace</strong><small>学习环境 · UID {{ uid }}</small></div>
        <span class="chevron">⌄</span>
      </div>
      <nav class="nav-list">
        <button v-for="item in navItems" :key="item.key" class="nav-item" :class="{ active: activeView === item.key }" @click="activeView = item.key">
          <span class="nav-icon">{{ item.icon }}</span><span class="nav-copy"><b>{{ item.label }}</b><small>{{ item.hint }}</small></span>
          <span v-if="item.key === 'executions' && activeExecutions" class="nav-badge">{{ activeExecutions }}</span>
        </button>
      </nav>
      <div class="sidebar-bottom">
        <div class="learn-card">
          <div class="learn-icon">✦</div><strong>Learn by doing</strong>
          <p>从定义节点，到观察一次真实的 Temporal 执行。</p>
          <button @click="activeView = 'playground'">打开调试台 <span>→</span></button>
        </div>
        <button class="settings-button" @click="settingsOpen = true"><span>⚙</span> Connection settings</button>
      </div>
    </aside>

    <main class="main-area">
      <header class="topbar">
        <div class="breadcrumbs"><span>Workspace</span><i>/</i><strong>{{ pageTitle }}</strong></div>
        <div class="topbar-actions">
          <div class="live-status"><span class="pulse" :class="statusClass(apiHealth)"></span>{{ apiHealth === 'up' ? 'Live data' : statusLabel(apiHealth) }}</div>
          <button class="icon-button" title="刷新" :class="{ spinning: refreshing }" @click="refreshAll()">↻</button>
          <button class="avatar-button" @click="settingsOpen = true">L</button>
        </div>
      </header>

      <div class="content-wrap">
        <section v-if="activeView === 'overview'" class="view-section">
          <div class="page-intro hero-intro">
            <div><div class="eyebrow"><span class="eyebrow-dot"></span> LOCAL CONTROL ROOM / {{ new Date().toLocaleDateString('zh-CN') }}</div><h1>Observe. Trigger. Learn.</h1><p>用一个控制台理解工作流从定义、调度到完成的完整路径。</p></div>
            <button class="primary-button" @click="openCreate"><span>＋</span> New workflow</button>
          </div>
          <div class="metric-grid">
            <article class="metric-card accent-purple"><div class="metric-heading"><span class="metric-icon">◇</span><span>工作流</span><span class="trend">+{{ workflows.length ? '100' : '0' }}%</span></div><strong>{{ workflows.length }}</strong><small>当前 UID 的定义</small><div class="sparkline purple"><i></i><i></i><i></i><i></i><i></i><i></i><i></i></div></article>
            <article class="metric-card accent-yellow"><div class="metric-heading"><span class="metric-icon">↗</span><span>进行中</span><span class="metric-live"><span></span> live</span></div><strong>{{ activeExecutions }}</strong><small>等待或正在执行</small><div class="sparkline yellow"><i></i><i></i><i></i><i></i><i></i><i></i><i></i></div></article>
            <article class="metric-card accent-green"><div class="metric-heading"><span class="metric-icon">✓</span><span>已完成</span><span class="trend">健康</span></div><strong>{{ completedExecutions }}</strong><small>最近一批执行</small><div class="sparkline green"><i></i><i></i><i></i><i></i><i></i><i></i><i></i></div></article>
            <article class="metric-card accent-red"><div class="metric-heading"><span class="metric-icon">!</span><span>需排查</span><span class="trend danger-text">attention</span></div><strong>{{ failedExecutions }}</strong><small>失败或取消的执行</small><div class="sparkline red"><i></i><i></i><i></i><i></i><i></i><i></i><i></i></div></article>
          </div>
          <div class="overview-grid">
            <article class="panel recent-panel"><div class="panel-header"><div><div class="section-kicker">ACTIVITY STREAM</div><h2>最近执行</h2></div><button class="text-button" @click="activeView = 'executions'">查看全部 <span>→</span></button></div><div v-if="recentExecutions.length" class="activity-list"><button v-for="execution in recentExecutions" :key="execution.execution_id" class="activity-row" @click="showExecution(execution); activeView = 'executions'"><span class="activity-status" :class="statusClass(execution.status)"></span><span class="activity-main"><strong>{{ shortId(execution.execution_id, 22) }}</strong><small>{{ execution.workflow_type || 'DynamicWorkflow' }} · {{ formatDate(execution.updated_at || execution.created_at) }}</small></span><span class="activity-value">{{ statusLabel(execution.status) }}</span><span class="row-arrow">↗</span></button></div><div v-else class="empty-state"><span>◌</span><strong>还没有执行记录</strong><p>从一个工作流开始，观察它如何进入 Temporal。</p><button class="secondary-button" @click="activeView = 'workflows'">选择工作流</button></div></article>
            <article class="panel health-panel"><div class="panel-header"><div><div class="section-kicker">SYSTEM PULSE</div><h2>服务状态</h2></div><span class="health-time">刚刚</span></div><div class="health-stack"><div class="health-row"><span class="health-symbol">◎</span><div><strong>Workflow API</strong><small>HTTP :8080</small></div><span class="health-pill" :class="statusClass(apiHealth)"><i></i>{{ statusLabel(apiHealth) }}</span></div><div class="health-row"><span class="health-symbol worker">✺</span><div><strong>Worker</strong><small>HTTP :8081 · Temporal worker</small></div><span class="health-pill" :class="statusClass(workerHealth)"><i></i>{{ statusLabel(workerHealth) }}</span></div><div class="health-row"><span class="health-symbol temporal">◒</span><div><strong>Temporal UI</strong><small>Web :18082</small></div><button class="small-link" @click="activeView = 'temporal'">打开 →</button></div></div><div class="health-note"><span>⌁</span><p>API 负责保存定义和创建执行，Worker 负责真正消费 Temporal 任务。</p></div></article>
          </div>
          <div class="panel learning-strip"><div class="learning-number">01</div><div><div class="section-kicker">A QUICK MENTAL MODEL</div><h2>先在这里理解一次执行</h2><p>Workflow 定义存入 PostgreSQL → API 创建 Execution → Temporal 调度 DynamicWorkflow → Worker 执行节点 → Execution 回写状态。</p></div><button class="secondary-button" @click="activeView = 'nodes'">探索节点 <span>↗</span></button></div>
        </section>

        <section v-else-if="activeView === 'workflows'" class="view-section">
          <div class="page-intro"><div><div class="eyebrow"><span class="eyebrow-dot purple-dot"></span> DEFINITIONS</div><h1>Workflows</h1><p>保存可重复运行的图定义，并用真实输入触发一次执行。</p></div><button class="primary-button" @click="openCreate"><span>＋</span> New workflow</button></div>
          <div class="toolbar"><div class="search-box"><span>⌕</span><input v-model="workflowQuery" placeholder="搜索名称、类型或 ID" /></div><span class="toolbar-count">{{ filteredWorkflows.length }} definitions</span></div>
          <div v-if="filteredWorkflows.length" class="workflow-grid"><article v-for="workflow in filteredWorkflows" :key="workflow.workflow_id" class="workflow-card"><div class="card-topline"><span class="type-badge">{{ workflow.type }}</span><span class="card-menu">•••</span></div><h3>{{ workflow.name }}</h3><p class="mono-id">{{ shortId(workflow.workflow_id, 28) }}</p><div class="workflow-graph-preview"><span v-for="(node, index) in (workflow.definition?.nodes || []).slice(0, 4)" :key="node.id" class="mini-node" :class="{ last: index === (workflow.definition?.nodes || []).slice(0, 4).length - 1 }"><i>{{ node.type.slice(0, 2) }}</i><b>{{ node.id }}</b></span><span v-if="(workflow.definition?.nodes || []).length > 4" class="more-nodes">+{{ (workflow.definition?.nodes || []).length - 4 }}</span></div><div class="card-meta"><span>{{ workflow.definition?.nodes?.length || 0 }} nodes</span><span>{{ formatDate(workflow.updated_at || workflow.created_at) }}</span></div><div class="card-actions"><button class="secondary-button compact" @click="showWorkflow(workflow)">Inspect</button><button class="primary-button compact" @click="openRun(workflow)">Run workflow <span>↗</span></button></div></article></div><div v-else class="panel empty-state large-empty"><span class="empty-illustration">◇</span><h2>创建第一个工作流</h2><p>从一个节点图开始，你可以马上在 Temporal UI 里看到它的执行。</p><button class="primary-button" @click="openCreate">Create a workflow</button></div>
          <div v-if="selectedWorkflow" class="detail-drawer"><div class="drawer-header"><div><div class="section-kicker">WORKFLOW INSPECTOR</div><h2>{{ selectedWorkflow.name }}</h2></div><button class="close-button" @click="selectedWorkflow = null">×</button></div><div class="drawer-meta"><span class="type-badge">{{ selectedWorkflow.type }}</span><span>{{ selectedWorkflow.workflow_id }}</span></div><div class="drawer-section"><h4>Definition graph</h4><div class="definition-list"><div v-for="node in selectedWorkflow.definition?.nodes || []" :key="node.id" class="definition-row"><span class="node-dot">{{ node.type.slice(0, 2) }}</span><span><strong>{{ node.id }}</strong><small>{{ node.type }}</small></span><code>{{ Object.keys(node.preset || {}).length }} preset keys</code></div></div></div><div class="drawer-section"><h4>Edges <span>{{ selectedWorkflow.definition?.edges?.length || 0 }}</span></h4><pre class="code-block">{{ jsonPretty(selectedWorkflow.definition?.edges || []) }}</pre></div><button class="primary-button full-button" @click="runSelectedWorkflow">Run this workflow <span>↗</span></button></div>
        </section>

        <section v-else-if="activeView === 'executions'" class="view-section">
          <div class="page-intro"><div><div class="eyebrow"><span class="eyebrow-dot yellow-dot"></span> OBSERVABILITY</div><h1>Executions</h1><p>定位失败节点、查看输入输出，并跳到 Temporal 查看完整历史。</p></div><div class="intro-actions"><button class="secondary-button" @click="refreshAll()">↻ Refresh</button><button class="primary-button" @click="activeView = 'workflows'">Run from workflow <span>↗</span></button></div></div>
          <div class="filter-tabs"><button v-for="filter in ['all', 'running', 'completed', 'failed']" :key="filter" :class="{ active: executionFilter === filter }" @click="executionFilter = filter">{{ filter === 'all' ? 'All executions' : statusLabel(filter) }} <span>{{ filter === 'all' ? executions.length : filter === 'running' ? activeExecutions : filter === 'completed' ? completedExecutions : failedExecutions }}</span></button></div>
          <div class="panel table-panel"><div class="table-head"><span>Execution</span><span>Workflow</span><span>Status</span><span>Updated</span><span></span></div><button v-for="execution in filteredExecutions" :key="execution.execution_id" class="table-row" :class="{ selected: selectedExecution?.execution_id === execution.execution_id }" @click="showExecution(execution)"><span class="table-id"><span class="activity-status" :class="statusClass(execution.status)"></span><strong>{{ shortId(execution.execution_id, 24) }}</strong></span><span><strong>{{ execution.workflow_type || 'DynamicWorkflow' }}</strong><small>{{ shortId(execution.workflow_id, 18) }}</small></span><span><span class="status-text" :class="statusClass(execution.status)">{{ statusLabel(execution.status) }}</span></span><span class="muted-text">{{ formatDate(execution.updated_at || execution.created_at) }}</span><span class="row-arrow">↗</span></button><div v-if="!filteredExecutions.length" class="table-empty">没有匹配的执行记录。</div></div>
          <div v-if="selectedExecution" class="execution-detail panel"><div class="detail-heading"><div><div class="section-kicker">EXECUTION DETAIL</div><h2>{{ shortId(selectedExecution.execution_id, 30) }}</h2><p>{{ selectedExecution.workflow_type || 'DynamicWorkflow' }} · {{ selectedExecution.workflow_id }}</p></div><span class="status-pill" :class="statusClass(selectedExecution.status)"><i></i>{{ statusLabel(selectedExecution.status) }}</span></div><div class="detail-tabs"><button :class="{ active: detailTab === 'input' }" @click="detailTab = 'input'">Input</button><button :class="{ active: detailTab === 'output' }" @click="detailTab = 'output'">Output</button><button :class="{ active: detailTab === 'timeline' }" @click="detailTab = 'timeline'">Timeline</button></div><pre v-if="detailTab === 'input'" class="code-block detail-code">{{ jsonPretty(selectedExecution.input) }}</pre><pre v-else-if="detailTab === 'output'" class="code-block detail-code">{{ jsonPretty(selectedExecution.output) }}</pre><div v-else class="timeline"><div><span class="timeline-dot done"></span><span><strong>Created</strong><small>{{ formatDate(selectedExecution.created_at) }}</small></span></div><div><span class="timeline-dot" :class="selectedExecution.started_at ? 'done' : ''"></span><span><strong>Started</strong><small>{{ formatDate(selectedExecution.started_at) }}</small></span></div><div><span class="timeline-dot" :class="selectedExecution.completed_at ? 'done' : ''"></span><span><strong>Completed</strong><small>{{ formatDate(selectedExecution.completed_at) }}</small></span></div></div><div v-if="selectedExecution.failed_node_id || selectedExecution.internal_error_message" class="error-callout"><strong>Failure signal</strong><span>{{ selectedExecution.failed_node_id ? `node: ${selectedExecution.failed_node_id}` : '' }} {{ selectedExecution.internal_error_message || '' }}</span></div><div class="detail-actions"><button class="secondary-button" @click="usePlaygroundExample(`/internal/v1/executions/${selectedExecution?.execution_id}`)">Open in API playground ↗</button><a class="secondary-button" :href="temporalUrl" target="_blank">Open Temporal UI ↗</a></div></div>
        </section>

        <section v-else-if="activeView === 'temporal'" class="view-section temporal-view"><div class="page-intro"><div><div class="eyebrow"><span class="eyebrow-dot temporal-dot"></span> NATIVE OBSERVABILITY</div><h1>Temporal UI</h1><p>在同一个工作台里查看 Namespace、Workflow History、Activity 与重试。</p></div><a class="primary-button" :href="temporalUrl" target="_blank">Open in new tab <span>↗</span></a></div><div class="temporal-toolbar"><div><span class="temporal-orb">◒</span><strong>Temporal Web</strong><span class="url-chip">{{ temporalUrl }}</span></div><span class="toolbar-help">如果浏览器拒绝 iframe 嵌入，请使用右上角新窗口打开。</span></div><div class="temporal-frame"><iframe :src="temporalUrl" title="Temporal UI"></iframe><div class="frame-overlay"><span>↗</span><p>Temporal UI 加载中…</p></div></div></section>

        <section v-else-if="activeView === 'nodes'" class="view-section"><div class="page-intro"><div><div class="eyebrow"><span class="eyebrow-dot green-dot"></span> BUILDING BLOCKS</div><h1>Node catalog</h1><p>这些节点来自 Worker 注册表，是工作流 Definition 可以使用的执行单元。</p></div><span class="count-bubble">{{ nodeTypes.length }} node types</span></div><div class="node-guide panel"><div class="guide-icon">✦</div><div><strong>如何读一个节点？</strong><p>Inputs 是上游边传入的字段，Preset 是创建工作流时固定的配置，Outputs 会成为下游节点的输入。</p></div><button class="secondary-button" @click="usePlaygroundExample('/internal/v1/node-types')">查看原始 JSON ↗</button></div><div class="node-grid"><article v-for="node in nodeTypes" :key="node.type" class="node-card"><div class="node-card-header"><span class="node-avatar">{{ node.type.slice(0, 2) }}</span><span class="type-badge">{{ node.type }}</span></div><div class="port-group"><h4>Inputs <span>{{ Object.keys(node.inputs || {}).length }}</span></h4><div v-if="Object.keys(node.inputs || {}).length" v-for="(input, name) in node.inputs" :key="name" class="port-row"><span class="port-dot input"></span><code>{{ name }}</code><small>{{ input.type }}{{ input.required ? ' · required' : '' }}</small></div><small v-else class="no-port">无输入端口</small></div><div class="port-group"><h4>Outputs <span>{{ Object.keys(node.outputs || {}).length }}</span></h4><div v-if="Object.keys(node.outputs || {}).length" v-for="(output, name) in node.outputs" :key="name" class="port-row"><span class="port-dot output"></span><code>{{ name }}</code><small>{{ output.type }}</small></div><small v-else class="no-port">无输出端口</small></div></article></div></section>

        <section v-else class="view-section"><div class="page-intro"><div><div class="eyebrow"><span class="eyebrow-dot red-dot"></span> REQUEST INSPECTOR</div><h1>API playground</h1><p>把 curl 搬到这里：发送请求、保留 JSON、快速复现一个问题。</p></div><span class="method-hint">Base: {{ workflowBase || 'Vite proxy → :8080' }}</span></div><div class="playground-layout"><div class="panel request-panel"><div class="panel-header"><div><div class="section-kicker">REQUEST</div><h2>Compose request</h2></div><span class="request-dot"></span></div><div class="request-line"><select v-model="playgroundMethod"><option>GET</option><option>POST</option></select><input v-model="playgroundPath" spellcheck="false" /><button class="primary-button compact" :disabled="playgroundBusy" @click="runPlayground">{{ playgroundBusy ? 'Sending…' : 'Send ↗' }}</button></div><label v-if="playgroundMethod !== 'GET'">JSON body<textarea v-model="playgroundBody" spellcheck="false" placeholder="{\n  &quot;uid&quot;: 1\n}"></textarea></label><div class="example-list"><div class="section-kicker">STARTER REQUESTS</div><button @click="usePlaygroundExample('/live')"><span class="method-get">GET</span><b>/live</b><small>Workflow API health</small></button><button @click="usePlaygroundExample('/internal/v1/node-types')"><span class="method-get">GET</span><b>/internal/v1/node-types</b><small>Node schemas</small></button><button @click="usePlaygroundExample('/internal/v1/executions?uid=1&limit=20&offset=0')"><span class="method-get">GET</span><b>/internal/v1/executions</b><small>Recent executions</small></button></div></div><div class="panel response-panel"><div class="panel-header"><div><div class="section-kicker">RESPONSE</div><h2>Inspect payload</h2></div><button class="text-button" @click="apiResponse = ''">Clear</button></div><pre v-if="apiResponse" class="code-block response-code">{{ apiResponse }}</pre><div v-else class="response-empty"><span>⌘</span><strong>Response appears here</strong><p>选择左侧 starter request，或输入你自己的路径。</p></div></div></div></section>
      </div>
    </main>

    <div v-if="toast" class="toast" :class="toastKind"><span>{{ toastKind === 'success' ? '✓' : toastKind === 'error' ? '!' : 'i' }}</span>{{ toast }}</div>

    <div v-if="createOpen" class="modal-backdrop" @click.self="createOpen = false"><div class="modal modal-wide"><div class="modal-header"><div><div class="section-kicker">NEW DEFINITION</div><h2>Create workflow</h2><p>先保存图定义，再从 Workflows 页面触发它。</p></div><button class="close-button" @click="createOpen = false">×</button></div><div class="form-grid"><label>名称<input v-model="newWorkflowName" placeholder="例如：图像生成示例" /></label><label>类型<input v-model="newWorkflowType" placeholder="template" /></label></div><label>Definition JSON<textarea v-model="newWorkflowDefinition" class="large-textarea" spellcheck="false"></textarea></label><div class="modal-footer"><span class="form-tip">节点类型可在 Node catalog 查看。</span><button class="secondary-button" @click="createOpen = false">Cancel</button><button class="primary-button" @click="submitWorkflow">Create workflow <span>↗</span></button></div></div></div>
    <div v-if="runOpen" class="modal-backdrop" @click.self="runOpen = false"><div class="modal"><div class="modal-header"><div><div class="section-kicker">TRIGGER EXECUTION</div><h2>Run workflow</h2><p>{{ selectedWorkflow?.name }}</p></div><button class="close-button" @click="runOpen = false">×</button></div><div class="run-summary"><span class="workflow-symbol">◇</span><div><strong>{{ selectedWorkflow?.name }}</strong><small>{{ shortId(selectedWorkflow?.workflow_id, 30) }}</small></div><span class="type-badge">{{ selectedWorkflow?.type }}</span></div><label>Input JSON<textarea v-model="runInput" class="run-textarea" spellcheck="false"></textarea></label><div class="modal-footer"><span class="form-tip">会生成新的 idempotency key。</span><button class="secondary-button" @click="runOpen = false">Cancel</button><button class="primary-button" @click="submitRun">Start execution <span>↗</span></button></div></div></div>
    <div v-if="settingsOpen" class="modal-backdrop" @click.self="settingsOpen = false"><div class="modal settings-modal"><div class="modal-header"><div><div class="section-kicker">LOCAL CONNECTION</div><h2>Connection settings</h2><p>默认空值使用 Vite 代理，适合本地 Docker 环境。</p></div><button class="close-button" @click="settingsOpen = false">×</button></div><label>UID<input v-model.number="uid" type="number" min="1" /></label><label>Workflow API base URL<input v-model="workflowBase" placeholder="留空：/ → localhost:8080" /></label><label>Worker API base URL<input v-model="workerBase" placeholder="/worker → localhost:8081" /></label><label>Temporal UI URL<input v-model="temporalUrl" placeholder="http://127.0.0.1:18082/namespaces/default/workflows" /></label><div class="settings-note"><span>i</span><p>如果把前端部署到其他地方，再填入完整 URL，并确保 API 开启 CORS。</p></div><div class="modal-footer"><span></span><button class="secondary-button" @click="settingsOpen = false">Cancel</button><button class="primary-button" @click="persistSettings">Save settings</button></div></div></div>
  </div>
</template>
