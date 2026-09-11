import { apiClient } from './client'

export const MODEL_EVALUATION_PROMPT = '生成html，内容是svg绘制鹈鹕骑自行车2D动画，不用进行测试。'
export type ModelEvaluationAPIFormat = 'chat_completions' | 'responses' | 'messages'
export type ModelEvaluationReasoningEffort = 'none' | 'minimal' | 'low' | 'medium' | 'high' | 'xhigh' | 'max' | 'ultra'
export interface ModelEvaluationGroup { id: number; name: string }
export interface ModelEvaluationTaskInput {
  name: string
  group_id: number
  endpoint: string
  api_format: ModelEvaluationAPIFormat
  api_key?: string
  model: string
  /** Optional reasoning intensity for the private test/scheduled request. */
  reasoning_effort?: ModelEvaluationReasoningEffort | ''
  enabled: boolean
  interval_seconds: number
  retention_days: number
  max_records: number
}
export interface ModelEvaluationTask extends Omit<ModelEvaluationTaskInput, 'api_key'> {
  id: number
  group_name: string
  has_api_key: boolean
  next_run_at: string | null
  created_at: string
  updated_at: string
  published: boolean
  test_status: 'untested' | 'running' | 'passed' | 'failed'
  last_tested_at: string | null
  test_error: string
}
export interface ModelEvaluationResult {
  id: number
  task_id: number
  task_name: string
  group_id: number
  group_name: string
  model: string
  status: 'success' | 'error'
  error_message?: string
  duration_ms: number
  created_at: string
  html?: string
  is_test?: boolean
}
export interface ModelEvaluationPage {
  items: ModelEvaluationResult[]
  total: number
  page: number
  page_size: number
}
export interface ModelEvaluationQuery { group_id?: number; task_id?: number; page: number; page_size: number }
const userPath = '/model-evaluations'
const adminPath = '/admin/model-evaluations'

export const modelEvaluationsAPI = {
  async groups(signal?: AbortSignal): Promise<{ enabled: boolean; items: ModelEvaluationGroup[] }> {
    return (await apiClient.get(`${userPath}/groups`, { signal })).data
  },
  async results(params: ModelEvaluationQuery, signal?: AbortSignal, admin = false): Promise<ModelEvaluationPage> {
    return (await apiClient.get(`${admin ? adminPath : userPath}/results`, { params, signal })).data
  },
  async result(id: number, signal?: AbortSignal, admin = false): Promise<ModelEvaluationResult> {
    return (await apiClient.get(`${admin ? adminPath : userPath}/results/${id}`, { signal })).data
  },
}

export const adminModelEvaluationsAPI = {
  async config(): Promise<{ enabled: boolean }> { return (await apiClient.get(`${adminPath}/config`)).data },
  async setConfig(enabled: boolean): Promise<{ enabled: boolean }> {
    return (await apiClient.put(`${adminPath}/config`, { enabled })).data
  },
  async tasks(signal?: AbortSignal): Promise<{ items: ModelEvaluationTask[] }> { return (await apiClient.get(`${adminPath}/tasks`, { signal })).data },
  async create(input: ModelEvaluationTaskInput): Promise<ModelEvaluationTask> {
    return (await apiClient.post(`${adminPath}/tasks`, input)).data
  },
  async update(id: number, input: ModelEvaluationTaskInput): Promise<ModelEvaluationTask> {
    return (await apiClient.put(`${adminPath}/tasks/${id}`, input)).data
  },
  async deleteTask(id: number): Promise<void> { await apiClient.delete(`${adminPath}/tasks/${id}`) },
  async run(id: number): Promise<void> { await apiClient.post(`${adminPath}/tasks/${id}/run`) },
  async test(id: number): Promise<void> { await apiClient.post(`${adminPath}/tasks/${id}/test`) },
  async setPublication(id: number, published: boolean): Promise<ModelEvaluationTask> {
    return (await apiClient.put(`${adminPath}/tasks/${id}/publication`, { published })).data
  },
  async deleteResult(id: number): Promise<void> { await apiClient.delete(`${adminPath}/results/${id}`) },
  async cleanup(input: { task_id?: number; all: boolean }): Promise<{ deleted: number }> {
    return (await apiClient.post(`${adminPath}/cleanup`, input)).data
  },
}
