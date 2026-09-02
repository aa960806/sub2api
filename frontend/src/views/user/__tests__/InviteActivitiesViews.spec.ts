import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import InviteLotteryView from '@/views/user/InviteLotteryView.vue'
import RechargeWheelView from '@/views/user/RechargeWheelView.vue'
import InviteMilestoneView from '@/views/user/InviteMilestoneView.vue'

const api = vi.hoisted(() => ({
  getInviteLotteryStatus: vi.fn(),
  claimInviteLottery: vi.fn(),
  getRechargeWheelStatus: vi.fn(),
  claimRechargeWheel: vi.fn(),
  getInviteMilestoneStatus: vi.fn(),
  claimInviteMilestone: vi.fn(),
}))
const showSuccess = vi.hoisted(() => vi.fn())
const showError = vi.hoisted(() => vi.fn())
const push = vi.hoisted(() => vi.fn())
const appStore = vi.hoisted(() => ({
  publicSettingsLoaded: true,
  cachedPublicSettings: {
    subnexus_invite_activities_enabled: true,
    subnexus_invite_lottery_enabled: true,
    subnexus_recharge_wheel_enabled: true,
    subnexus_invite_milestone_enabled: true,
  } as Record<string, boolean> | null,
  fetchPublicSettings: vi.fn(),
  showSuccess,
  showError,
}))

vi.mock('@/api/inviteActivities', () => api)
vi.mock('@/stores', () => ({ useAppStore: () => appStore }))
vi.mock('vue-router', () => ({ useRouter: () => ({ push }) }))
vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return { ...actual, useI18n: () => ({ t: (key: string) => key }) }
})

const global = {
  stubs: {
    AppLayout: { template: '<main><slot /></main>' },
    Icon: { template: '<i />' },
  },
}

function enabledSettings(): Record<string, boolean> {
  return {
    subnexus_invite_activities_enabled: true,
    subnexus_invite_lottery_enabled: true,
    subnexus_recharge_wheel_enabled: true,
    subnexus_invite_milestone_enabled: true,
  }
}

describe('invite activity user views', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    appStore.publicSettingsLoaded = true
    appStore.cachedPublicSettings = enabledSettings()
    api.getInviteLotteryStatus.mockResolvedValue({
      enabled: true,
      invited_count: 2,
      qualified_invited_count: 2,
      used_chances: 1,
      remaining_chances: 1,
      locked_chances: 0,
      recharge_limit_enabled: false,
      invitee_recharge_threshold: 10,
      can_claim: true,
      prizes: [{ name: 'Prize', amount: 1 }],
    })
    api.getRechargeWheelStatus.mockResolvedValue({
      enabled: true,
      threshold: 10,
      recharged_amount: 20,
      total_chances: 2,
      used_chances: 1,
      remaining_chances: 1,
      can_claim: true,
      amounts: [{ amount: 1 }],
      multipliers: [{ multiplier: 2 }],
    })
    api.getInviteMilestoneStatus.mockResolvedValue({
      enabled: true,
      invited_count: 5,
      qualified_invited_count: 5,
      recharge_limit_enabled: false,
      invitee_recharge_threshold: 10,
      tiers: [{ invites: 5, reward: 3, reached: true, recharge_reached: true, blocked_by_recharge: false, claimed: false, claimable: true }],
    })
  })

  it('does not query any activity endpoint when the aggregate flag is off', async () => {
    appStore.cachedPublicSettings = { ...enabledSettings(), subnexus_invite_activities_enabled: false }

    const lottery = mount(InviteLotteryView, { global })
    const wheel = mount(RechargeWheelView, { global })
    const milestone = mount(InviteMilestoneView, { global })
    await flushPromises()

    expect(lottery.find('[data-testid="invite-lottery-disabled"]').exists()).toBe(true)
    expect(wheel.find('[data-testid="recharge-wheel-disabled"]').exists()).toBe(true)
    expect(milestone.find('[data-testid="invite-milestone-disabled"]').exists()).toBe(true)
    expect(api.getInviteLotteryStatus).not.toHaveBeenCalled()
    expect(api.getRechargeWheelStatus).not.toHaveBeenCalled()
    expect(api.getInviteMilestoneStatus).not.toHaveBeenCalled()
  })

  it('claims an invite lottery reward from the enabled action', async () => {
    api.claimInviteLottery.mockResolvedValue({
      ...(await api.getInviteLotteryStatus()),
      remaining_chances: 0,
      can_claim: false,
      prize: { name: 'Prize', amount: 1 },
    })
    const wrapper = mount(InviteLotteryView, { global })
    await flushPromises()

    await wrapper.get('[data-testid="invite-lottery-claim"]').trigger('click')
    await flushPromises()

    expect(api.claimInviteLottery).toHaveBeenCalledOnce()
    expect(wrapper.find('[data-testid="invite-lottery-result"]').exists()).toBe(true)
  })

  it('claims a recharge wheel result from the enabled action', async () => {
    api.claimRechargeWheel.mockResolvedValue({
      ...(await api.getRechargeWheelStatus()),
      remaining_chances: 0,
      can_claim: false,
      result: { amount: 1, multiplier: 2, total: 2, amount_index: 0, multiplier_index: 0 },
    })
    const wrapper = mount(RechargeWheelView, { global })
    await flushPromises()

    await wrapper.get('[data-testid="recharge-wheel-claim"]').trigger('click')
    await flushPromises()

    expect(api.claimRechargeWheel).toHaveBeenCalledOnce()
    expect(wrapper.find('[data-testid="recharge-wheel-result"]').exists()).toBe(true)
  })

  it('passes the selected invite target when claiming a milestone', async () => {
    api.claimInviteMilestone.mockResolvedValue({
      ...(await api.getInviteMilestoneStatus()),
      just_claimed_reward: 3,
      tiers: [{ invites: 5, reward: 3, reached: true, recharge_reached: true, blocked_by_recharge: false, claimed: true, claimable: false }],
    })
    const wrapper = mount(InviteMilestoneView, { global })
    await flushPromises()

    await wrapper.get('[data-testid="invite-milestone-claim-5"]').trigger('click')
    await flushPromises()

    expect(api.claimInviteMilestone).toHaveBeenCalledWith(5)
  })
})
