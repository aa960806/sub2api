/**
 * SubNexus leaderboard API.
 *
 * The backend exposes these as read-only aggregates. The independent public
 * feature flag is checked by the route/view before requests are issued.
 */
import { apiClient } from './client'

export type LeaderboardWindow = 'today' | 'week' | 'month'

export interface LeaderboardEntry {
  rank: number
  user_id: number
  email: string
  usage: number
  requests: number
  tokens: number
  reward_amount?: number
  rewarded: boolean
}

export interface LeaderboardResponse {
  window: LeaderboardWindow | string
  title: string
  start_date: string
  end_date: string
  total_usage: number
  requests: number
  tokens: number
  reward_top_n: number
  reward_value: number
  reward_amounts: number[]
  entries: LeaderboardEntry[]
}

export interface InviteLeaderboardEntry {
  rank: number
  user_id: number
  email: string
  invite_count: number
}

export interface InviteLeaderboardResponse {
  window: LeaderboardWindow | string
  title: string
  start_date: string
  end_date: string
  total_invites: number
  entries: InviteLeaderboardEntry[]
}

export interface LeaderboardConfig {
  enabled: boolean
  weekly_enabled: boolean
  weekly_top_n: number
  weekly_reward: number
  weekly_rewards: number[]
  monthly_enabled: boolean
  monthly_top_n: number
  monthly_reward: number
  monthly_rewards: number[]
}

export async function getLeaderboard(
  window: LeaderboardWindow = 'week',
  limit = 20,
): Promise<LeaderboardResponse> {
  const { data } = await apiClient.get<LeaderboardResponse>('/activity/leaderboard', {
    params: { window, limit },
  })
  return data
}

export async function getInviteLeaderboard(
  window: LeaderboardWindow = 'week',
  limit = 20,
): Promise<InviteLeaderboardResponse> {
  const { data } = await apiClient.get<InviteLeaderboardResponse>('/activity/invite-leaderboard', {
    params: { window, limit },
  })
  return data
}

export async function getLeaderboardConfig(): Promise<LeaderboardConfig> {
  const { data } = await apiClient.get<LeaderboardConfig>('/admin/leaderboard/config')
  return data
}

export async function updateLeaderboardConfig(
  config: LeaderboardConfig,
): Promise<LeaderboardConfig> {
  const { data } = await apiClient.put<LeaderboardConfig>('/admin/leaderboard/config', config)
  return data
}

const leaderboardAPI = {
  get: getLeaderboard,
  getInvites: getInviteLeaderboard,
  getConfig: getLeaderboardConfig,
  updateConfig: updateLeaderboardConfig,
}

export default leaderboardAPI
