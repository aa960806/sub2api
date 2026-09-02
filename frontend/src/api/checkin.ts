/**
 * SubNexus daily check-in API.
 *
 * The server protects these endpoints with the independent
 * `subnexus_checkin_enabled` setting. Callers should still gate the UI with
 * the matching public feature flag so a disabled rollout performs no request.
 */
import { apiClient } from './client'

export type CheckInCycleMode = 'reset' | 'cumulative'
export type CheckInPaidMode = 'off' | 'limit' | 'hide'
export type CheckInOverLimitAction = 'prompt' | 'freeze'

export interface CheckInConfig {
  enabled: boolean
  checkin_ip_limit: boolean
  checkin_min: number
  checkin_max: number
  checkin_cycle_mode: CheckInCycleMode
  checkin_milestone_days: number
  checkin_milestone_min: number
  checkin_milestone_max: number
  checkin_paid_mode: CheckInPaidMode
  checkin_free_max_count: number
  checkin_free_max_amount: number
  checkin_over_limit_action: CheckInOverLimitAction
}

export interface CheckInStatus {
  enabled: boolean
  checked_in: boolean
  amount?: number
  min_amount: number
  max_amount: number
  streak: number
  next_streak: number
  rule_start_day: number
  rule_end_day: number
  cycle_mode: CheckInCycleMode
  milestone_days: number
  milestone: boolean
  continuous_streak: number
  cycle_day: number
  cycle_completed_days: number
  checked_at?: string
  next_at: string
  paid?: boolean
  locked?: boolean
  limit_reached?: boolean
  over_limit_action?: CheckInOverLimitAction | string
  frozen_amount?: number
  today_frozen?: boolean
}

export async function getCheckInStatus(): Promise<CheckInStatus> {
  const { data } = await apiClient.get<CheckInStatus>('/activity/checkin')
  return data
}

export async function claimCheckIn(): Promise<CheckInStatus> {
  const { data } = await apiClient.post<CheckInStatus>('/activity/checkin')
  return data
}

export async function getCheckInConfig(): Promise<CheckInConfig> {
  const { data } = await apiClient.get<CheckInConfig>('/admin/checkin/config')
  return data
}

export async function updateCheckInConfig(config: CheckInConfig): Promise<CheckInConfig> {
  const { data } = await apiClient.put<CheckInConfig>('/admin/checkin/config', config)
  return data
}

const checkInAPI = {
  getStatus: getCheckInStatus,
  claim: claimCheckIn,
  getConfig: getCheckInConfig,
  updateConfig: updateCheckInConfig,
}

export default checkInAPI
