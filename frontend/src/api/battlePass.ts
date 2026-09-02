import { apiClient } from './client'

export interface BattlePassSettings {
  enabled: boolean
  test_tools_enabled?: boolean
}

export interface BattlePassSeason {
  id: number
  name: string
  description: string
  status: string
  runtime_status: string
  timezone: string
  start_at: string
  end_at: string
  premium_price: number
  max_level: number
  user_side_enabled: boolean
}

export interface BattlePassLevelInput {
  level: number
  required_exp: number
}

export interface BattlePassTaskInput {
  name: string
  description: string
  task_type: string
  period_type: 'daily' | 'season'
  target_value: number
  exp_reward: number
  filter_scope: string
  filter_values: string[]
  display_order: number
  enabled: boolean
}

export interface BattlePassRewardInput {
  level: number
  track: 'free' | 'premium'
  reward_type: string
  payload: Record<string, unknown>
}

export interface BattlePassSeasonDraft {
  name: string
  description: string
  timezone: string
  start_at: string
  end_at: string
  premium_price: number
  max_level: number
  levels: BattlePassLevelInput[]
  tasks: BattlePassTaskInput[]
  rewards: BattlePassRewardInput[]
}

export interface BattlePassSeasonDetail extends BattlePassSeason, BattlePassSeasonDraft {}

export interface BattlePassValidationResult {
  ok: boolean
  errors: { level: string; code: string; message: string }[]
  warnings: { level: string; code: string; message: string }[]
}

export interface BattlePassCurrent {
  season: BattlePassSeason | null
  progress?: BattlePassUserProgress | null
  syncing: boolean
  user_side_enabled: boolean
}

export interface BattlePassUserProgress {
  exp: number
  level: number
  level_start_exp: number
  next_level_exp?: number | null
  premium_unlocked: boolean
  updated_at: string
}

export function battlePassLevelProgress(progress?: BattlePassUserProgress | null) {
  if (!progress) return 0
  const start = Number.isFinite(progress.level_start_exp) ? progress.level_start_exp : 0
  const next = progress.next_level_exp
  if (next == null || next <= start) return 100
  const exp = Number.isFinite(progress.exp) ? progress.exp : start
  return Math.max(0, Math.min(100, (exp - start) / (next - start) * 100))
}

export interface BattlePassTaskState extends BattlePassTaskInput {
  id: number
  current_value: number
  completed: boolean
  period_key: string
}

export interface BattlePassRewardState {
  id: number
  level: number
  track: 'free' | 'premium'
  reward_type: string
  payload: Record<string, unknown>
  status: 'locked' | 'premium_locked' | 'claimable' | 'pending' | 'processing' | 'granted' | 'granted_capped' | 'failed' | 'blocked_config'
  last_error?: string
}

export interface BattlePassClaimResult {
  claimed_count: number
  rewards: BattlePassRewardState[]
}

export interface BattlePassPurchase {
  id: number
  season_id: number
  price: number
  balance_before: number
  balance_after: number
  purchased_at: string
}

export interface BattlePassHistory {
  experience: Array<{ id: number; task_id?: number; period_key: string; exp_delta: number; reason: string; created_at: string }>
  purchases: BattlePassPurchase[]
  rewards: BattlePassRewardState[]
}

export interface BattlePassCosmetic {
  id: number
  kind: 'badge' | 'title'
  code: string
  name: string
  color_token: string
  asset_key: string
  equipped: boolean
  granted_at: string
}

export interface BattlePassTestUser {
  id: number
  email: string
}

export interface BattlePassTestState {
  season: BattlePassSeason
  user: BattlePassTestUser
  progress?: BattlePassUserProgress | null
  tasks: BattlePassTaskState[]
  rewards: BattlePassRewardState[]
}

export interface BattlePassTestCompleteResult {
  completed_count: number
  state: BattlePassTestState
}

export function getBattlePassCurrent() {
  return apiClient.get<BattlePassCurrent>('/battle-pass/current').then((res) => res.data)
}

export function getBattlePassTasks() {
  return apiClient.get<BattlePassTaskState[]>('/battle-pass/current/tasks').then((res) => res.data)
}

export function getBattlePassRewards() {
  return apiClient.get<BattlePassRewardState[]>('/battle-pass/current/rewards').then((res) => res.data)
}

export function claimBattlePassReward(rewardId: number) {
  return apiClient.post<BattlePassClaimResult>(`/battle-pass/current/rewards/${rewardId}/claim`).then((res) => res.data)
}

export function claimAllBattlePassRewards() {
  return apiClient.post<BattlePassClaimResult>('/battle-pass/current/rewards/claim-all').then((res) => res.data)
}

export function getBattlePassHistory(seasonId?: number) {
  return apiClient.get<BattlePassHistory>('/battle-pass/current/history', {
    params: seasonId && seasonId > 0 ? { season_id: seasonId } : undefined,
  }).then((res) => res.data)
}

export function purchaseBattlePass(idempotencyKey: string) {
  return apiClient.post<BattlePassPurchase>('/battle-pass/current/purchase', { idempotency_key: idempotencyKey }, {
    headers: { 'Idempotency-Key': idempotencyKey },
  }).then((res) => res.data)
}

export function getBattlePassCosmetics() {
  return apiClient.get<BattlePassCosmetic[]>('/battle-pass/cosmetics').then((res) => res.data)
}

export function equipBattlePassCosmetic(cosmeticId: number) {
  return apiClient.put<{ equipped: boolean }>('/battle-pass/cosmetics/equipped', { cosmetic_id: cosmeticId }).then((res) => res.data)
}

export function getBattlePassSettings() {
  return apiClient.get<BattlePassSettings>('/admin/activity/battle-pass/settings').then((res) => res.data)
}

export function updateBattlePassSettings(payload: BattlePassSettings) {
  return apiClient.put<BattlePassSettings>('/admin/activity/battle-pass/settings', payload).then((res) => res.data)
}

export function listBattlePassSeasons() {
  return apiClient.get<BattlePassSeason[]>('/admin/activity/battle-pass/seasons').then((res) => res.data)
}

export function getBattlePassSeason(id: number) {
  return apiClient.get<BattlePassSeasonDetail>(`/admin/activity/battle-pass/seasons/${id}`).then((res) => res.data)
}

export function createBattlePassSeason(payload: BattlePassSeasonDraft) {
  return apiClient.post<BattlePassSeasonDetail>('/admin/activity/battle-pass/seasons', payload).then((res) => res.data)
}

export function updateBattlePassSeason(id: number, payload: BattlePassSeasonDraft) {
  return apiClient.put<BattlePassSeasonDetail>(`/admin/activity/battle-pass/seasons/${id}`, payload).then((res) => res.data)
}

export function validateBattlePassSeason(id: number) {
  return apiClient.post<BattlePassValidationResult>(`/admin/activity/battle-pass/seasons/${id}/validate`).then((res) => res.data)
}

export function publishBattlePassSeason(id: number) {
  return apiClient.post<BattlePassSeasonDetail>(`/admin/activity/battle-pass/seasons/${id}/publish`).then((res) => res.data)
}

export function pauseBattlePassSeason(id: number) {
  return apiClient.post<BattlePassSeasonDetail>(`/admin/activity/battle-pass/seasons/${id}/pause`).then((res) => res.data)
}

export function resumeBattlePassSeason(id: number) {
  return apiClient.post<BattlePassSeasonDetail>(`/admin/activity/battle-pass/seasons/${id}/resume`).then((res) => res.data)
}

export function endBattlePassSeason(id: number) {
  return apiClient.post<BattlePassSeasonDetail>(`/admin/activity/battle-pass/seasons/${id}/end`).then((res) => res.data)
}

export function getBattlePassTestState(seasonId: number, userId: number) {
  return apiClient.get<BattlePassTestState>('/admin/activity/battle-pass/test/state', {
    params: { season_id: seasonId, user_id: userId },
  }).then((res) => res.data)
}

export function activateBattlePassSeasonForTest(seasonId: number) {
  return apiClient.post<BattlePassSeason>('/admin/activity/battle-pass/test/activate', { season_id: seasonId }).then((res) => res.data)
}

export function completeBattlePassTasksForTest(seasonId: number, userId: number, taskId = 0) {
  return apiClient.post<BattlePassTestCompleteResult>('/admin/activity/battle-pass/test/complete', {
    season_id: seasonId,
    user_id: userId,
    task_id: taskId,
  }).then((res) => res.data)
}
