import { beforeEach, describe, expect, it, vi } from 'vitest'

const { get, post, put } = vi.hoisted(() => ({
  get: vi.fn(),
  post: vi.fn(),
  put: vi.fn(),
}))

vi.mock('@/api/client', () => ({ apiClient: { get, post, put } }))

import {
  claimInviteLottery,
  claimInviteMilestone,
  claimRechargeWheel,
  getInviteActivitiesConfig,
  getInviteLotteryStatus,
  getInviteMilestoneStatus,
  getRechargeWheelStatus,
  updateInviteActivitiesConfig,
  type InviteActivitiesConfig,
} from '@/api/inviteActivities'

const config: InviteActivitiesConfig = {
  enabled: false,
  invite_lottery_enabled: false,
  invite_lottery_prizes: [{ name: 'Prize', amount: 1, probability: 100 }],
  invite_lottery_recharge_limit_enabled: false,
  invite_lottery_recharge_threshold: 10,
  recharge_wheel_enabled: false,
  recharge_wheel_threshold: 10,
  recharge_wheel_amounts: [{ amount: 1, probability: 100 }],
  recharge_wheel_multipliers: [{ multiplier: 2, probability: 100 }],
  invite_milestone_enabled: false,
  invite_milestone_tiers: [{ invites: 5, reward: 1 }],
  invite_milestone_recharge_limit_enabled: false,
  invite_milestone_recharge_threshold: 10,
}

describe('invite activities API', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    get.mockResolvedValue({ data: {} })
    post.mockResolvedValue({ data: {} })
    put.mockResolvedValue({ data: config })
  })

  it('uses the isolated user endpoints and exact milestone payload', async () => {
    await getInviteLotteryStatus()
    await claimInviteLottery()
    await getRechargeWheelStatus()
    await claimRechargeWheel()
    await getInviteMilestoneStatus()
    await claimInviteMilestone(25)

    expect(get).toHaveBeenNthCalledWith(1, '/activity/invite-lottery')
    expect(post).toHaveBeenNthCalledWith(1, '/activity/invite-lottery')
    expect(get).toHaveBeenNthCalledWith(2, '/activity/recharge-wheel')
    expect(post).toHaveBeenNthCalledWith(2, '/activity/recharge-wheel')
    expect(get).toHaveBeenNthCalledWith(3, '/activity/invite-milestone')
    expect(post).toHaveBeenNthCalledWith(3, '/activity/invite-milestone', { invites: 25 })
  })

  it('uses the independent administrator config endpoint', async () => {
    get.mockResolvedValueOnce({ data: config })
    await getInviteActivitiesConfig()
    await updateInviteActivitiesConfig(config)

    expect(get).toHaveBeenCalledWith('/admin/invite-activities/config')
    expect(put).toHaveBeenCalledWith('/admin/invite-activities/config', config)
  })
})
