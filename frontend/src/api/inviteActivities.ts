/**
 * SubNexus invite/recharge reward activities.
 *
 * The three user activities share one atomic administrator policy, but each
 * user entry has an independent public feature flag. Probability weights are
 * intentionally present only in the administrator configuration types.
 */
import { apiClient } from './client'

export interface InviteLotteryPrize {
  name: string
  amount: number
  probability: number
}

export interface InviteLotteryPrizePublic {
  name: string
  amount: number
}

export interface RechargeWheelAmount {
  amount: number
  probability: number
}

export interface RechargeWheelMultiplier {
  multiplier: number
  probability: number
}

export interface InviteMilestoneTier {
  invites: number
  reward: number
}

export interface InviteActivitiesConfig {
  enabled: boolean
  invite_lottery_enabled: boolean
  invite_lottery_prizes: InviteLotteryPrize[]
  invite_lottery_recharge_limit_enabled: boolean
  invite_lottery_recharge_threshold: number
  recharge_wheel_enabled: boolean
  recharge_wheel_threshold: number
  recharge_wheel_amounts: RechargeWheelAmount[]
  recharge_wheel_multipliers: RechargeWheelMultiplier[]
  invite_milestone_enabled: boolean
  invite_milestone_tiers: InviteMilestoneTier[]
  invite_milestone_recharge_limit_enabled: boolean
  invite_milestone_recharge_threshold: number
}

export interface InviteLotteryStatus {
  enabled: boolean
  invited_count: number
  qualified_invited_count: number
  used_chances: number
  remaining_chances: number
  locked_chances: number
  recharge_limit_enabled: boolean
  invitee_recharge_threshold: number
  can_claim: boolean
  prize?: InviteLotteryPrizePublic
  prizes: InviteLotteryPrizePublic[]
}

export interface RechargeWheelResult {
  amount: number
  multiplier: number
  total: number
  amount_index: number
  multiplier_index: number
}

export interface RechargeWheelStatus {
  enabled: boolean
  threshold: number
  recharged_amount: number
  total_chances: number
  used_chances: number
  remaining_chances: number
  can_claim: boolean
  result?: RechargeWheelResult
  amounts: Array<{ amount: number }>
  multipliers: Array<{ multiplier: number }>
}

export interface InviteMilestoneTierStatus {
  invites: number
  reward: number
  reached: boolean
  recharge_reached: boolean
  blocked_by_recharge: boolean
  claimed: boolean
  claimable: boolean
}

export interface InviteMilestoneStatus {
  enabled: boolean
  invited_count: number
  qualified_invited_count: number
  recharge_limit_enabled: boolean
  invitee_recharge_threshold: number
  just_claimed_reward?: number
  tiers: InviteMilestoneTierStatus[]
}

export async function getInviteLotteryStatus(): Promise<InviteLotteryStatus> {
  const { data } = await apiClient.get<InviteLotteryStatus>('/activity/invite-lottery')
  return data
}

export async function claimInviteLottery(): Promise<InviteLotteryStatus> {
  const { data } = await apiClient.post<InviteLotteryStatus>('/activity/invite-lottery')
  return data
}

export async function getRechargeWheelStatus(): Promise<RechargeWheelStatus> {
  const { data } = await apiClient.get<RechargeWheelStatus>('/activity/recharge-wheel')
  return data
}

export async function claimRechargeWheel(): Promise<RechargeWheelStatus> {
  const { data } = await apiClient.post<RechargeWheelStatus>('/activity/recharge-wheel')
  return data
}

export async function getInviteMilestoneStatus(): Promise<InviteMilestoneStatus> {
  const { data } = await apiClient.get<InviteMilestoneStatus>('/activity/invite-milestone')
  return data
}

export async function claimInviteMilestone(invites: number): Promise<InviteMilestoneStatus> {
  const { data } = await apiClient.post<InviteMilestoneStatus>('/activity/invite-milestone', { invites })
  return data
}

export async function getInviteActivitiesConfig(): Promise<InviteActivitiesConfig> {
  const { data } = await apiClient.get<InviteActivitiesConfig>('/admin/invite-activities/config')
  return data
}

export async function updateInviteActivitiesConfig(
  config: InviteActivitiesConfig,
): Promise<InviteActivitiesConfig> {
  const { data } = await apiClient.put<InviteActivitiesConfig>('/admin/invite-activities/config', config)
  return data
}

const inviteActivitiesAPI = {
  getInviteLotteryStatus,
  claimInviteLottery,
  getRechargeWheelStatus,
  claimRechargeWheel,
  getInviteMilestoneStatus,
  claimInviteMilestone,
  getConfig: getInviteActivitiesConfig,
  updateConfig: updateInviteActivitiesConfig,
}

export default inviteActivitiesAPI
