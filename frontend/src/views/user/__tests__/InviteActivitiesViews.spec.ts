import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
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
  return {
    ...actual,
    useI18n: () => ({ locale: { value: 'zh-CN' }, t: (key: string) => key }),
  }
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

function lotteryStatus() {
  return {
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
  }
}

function wheelStatus() {
  return {
    enabled: true,
    threshold: 10,
    recharged_amount: 20,
    total_chances: 2,
    used_chances: 1,
    remaining_chances: 1,
    can_claim: true,
    amounts: [{ amount: 1 }],
    multipliers: [{ multiplier: 2 }],
  }
}

function milestoneStatus() {
  return {
    enabled: true,
    invited_count: 5,
    qualified_invited_count: 5,
    recharge_limit_enabled: false,
    invitee_recharge_threshold: 10,
    tiers: [
      {
        invites: 5,
        reward: 3,
        reached: true,
        recharge_reached: true,
        blocked_by_recharge: false,
        claimed: false,
        claimable: true,
      },
    ],
  }
}

function lotteryPrizes(count: number) {
  return Array.from({ length: count }, (_, index) => ({
    name: `Prize ${index + 1}`,
    amount: index + 1,
  }))
}

function wheelAmounts(count: number) {
  return Array.from({ length: count }, (_, index) => ({ amount: index + 1 }))
}

function wheelMultipliers(count: number) {
  return Array.from({ length: count }, (_, index) => ({ multiplier: index + 1 }))
}

function milestoneTiers(count: number) {
  return Array.from({ length: count }, (_, index) => ({
    invites: (index + 1) ** 2,
    reward: index + 1,
    reached: false,
    recharge_reached: false,
    blocked_by_recharge: false,
    claimed: false,
    claimable: false,
  }))
}

function useAnimationTimers(): void {
  vi.useFakeTimers()
  let frameTime = performance.now()
  vi.stubGlobal(
    'requestAnimationFrame',
    (callback: FrameRequestCallback) => window.setTimeout(() => {
      frameTime += 16
      callback(frameTime)
    }, 16),
  )
  vi.stubGlobal('cancelAnimationFrame', (handle: number) => window.clearTimeout(handle))
}

function failNextPublicSettingsRefresh(): void {
  appStore.fetchPublicSettings.mockImplementationOnce(async () => {
    appStore.publicSettingsLoaded = false
    appStore.cachedPublicSettings = null
    return null
  })
}

describe('invite activity user views', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    appStore.publicSettingsLoaded = true
    appStore.cachedPublicSettings = enabledSettings()
    appStore.fetchPublicSettings.mockImplementation(async () => appStore.cachedPublicSettings)
    api.getInviteLotteryStatus.mockResolvedValue(lotteryStatus())
    api.getRechargeWheelStatus.mockResolvedValue(wheelStatus())
    api.getInviteMilestoneStatus.mockResolvedValue(milestoneStatus())
  })

  afterEach(() => {
    vi.useRealTimers()
    vi.unstubAllGlobals()
  })

  it('does not query any activity endpoint when the aggregate flag is off', async () => {
    appStore.cachedPublicSettings = {
      ...enabledSettings(),
      subnexus_invite_activities_enabled: false,
    }

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

    lottery.unmount()
    wheel.unmount()
    milestone.unmount()
  })

  it('fails closed for each disabled child activity switch', async () => {
    appStore.cachedPublicSettings = {
      ...enabledSettings(),
      subnexus_invite_lottery_enabled: false,
    }
    const lottery = mount(InviteLotteryView, { global })
    await flushPromises()
    expect(lottery.find('[data-testid="invite-lottery-disabled"]').exists()).toBe(true)
    expect(api.getInviteLotteryStatus).not.toHaveBeenCalled()
    lottery.unmount()

    appStore.cachedPublicSettings = {
      ...enabledSettings(),
      subnexus_recharge_wheel_enabled: false,
    }
    const wheel = mount(RechargeWheelView, { global })
    await flushPromises()
    expect(wheel.find('[data-testid="recharge-wheel-disabled"]').exists()).toBe(true)
    expect(api.getRechargeWheelStatus).not.toHaveBeenCalled()
    wheel.unmount()

    appStore.cachedPublicSettings = {
      ...enabledSettings(),
      subnexus_invite_milestone_enabled: false,
    }
    const milestone = mount(InviteMilestoneView, { global })
    await flushPromises()
    expect(milestone.find('[data-testid="invite-milestone-disabled"]').exists()).toBe(true)
    expect(api.getInviteMilestoneStatus).not.toHaveBeenCalled()
    milestone.unmount()
  })

  it('shows an isolated retry state when each activity status request fails', async () => {
    api.getInviteLotteryStatus.mockRejectedValueOnce(new Error('lottery unavailable'))
    const lottery = mount(InviteLotteryView, { global })
    await flushPromises()
    expect(lottery.find('[data-testid="invite-lottery-load-failed"]').exists()).toBe(true)
    expect(lottery.find('[data-testid="invite-lottery-claim"]').exists()).toBe(false)
    await lottery.get('[data-testid="invite-lottery-load-failed"] button').trigger('click')
    await flushPromises()
    expect(lottery.find('[data-testid="invite-lottery-load-failed"]').exists()).toBe(false)
    expect(lottery.find('[data-testid="invite-lottery-claim"]').exists()).toBe(true)
    lottery.unmount()

    api.getRechargeWheelStatus.mockRejectedValueOnce(new Error('wheel unavailable'))
    const wheel = mount(RechargeWheelView, { global })
    await flushPromises()
    expect(wheel.find('[data-testid="recharge-wheel-load-failed"]').exists()).toBe(true)
    expect(wheel.find('[data-testid="recharge-wheel-claim"]').exists()).toBe(false)
    await wheel.get('[data-testid="recharge-wheel-load-failed"] button').trigger('click')
    await flushPromises()
    expect(wheel.find('[data-testid="recharge-wheel-load-failed"]').exists()).toBe(false)
    expect(wheel.find('[data-testid="recharge-wheel-claim"]').exists()).toBe(true)
    wheel.unmount()

    api.getInviteMilestoneStatus.mockRejectedValueOnce(new Error('milestone unavailable'))
    const milestone = mount(InviteMilestoneView, { global })
    await flushPromises()
    expect(milestone.find('[data-testid="invite-milestone-load-failed"]').exists()).toBe(true)
    expect(milestone.find('[data-testid="invite-milestone-claim-5"]').exists()).toBe(false)
    await milestone.get('[data-testid="invite-milestone-load-failed"] button').trigger('click')
    await flushPromises()
    expect(milestone.find('[data-testid="invite-milestone-load-failed"]').exists()).toBe(false)
    expect(milestone.find('[data-testid="invite-milestone-claim-5"]').exists()).toBe(true)
    milestone.unmount()
  })

  it('keeps a 50-prize lottery readable and finishes its animation within 5.5 seconds', async () => {
    useAnimationTimers()
    const prizes = lotteryPrizes(50)
    api.getInviteLotteryStatus.mockResolvedValue({ ...lotteryStatus(), prizes })
    api.claimInviteLottery.mockResolvedValue({
      ...lotteryStatus(),
      remaining_chances: 0,
      can_claim: false,
      prizes,
      prize: prizes[49],
    })
    const wrapper = mount(InviteLotteryView, { global })
    await flushPromises()

    expect(wrapper.get('.lottery-stage').classes()).toContain('self-start')
    expect(wrapper.get('.lottery-board').classes()).toContain('lottery-board--dense')
    await wrapper.get('[data-testid="invite-lottery-claim"]').trigger('click')
    await flushPromises()
    await vi.advanceTimersByTimeAsync(5500)
    await flushPromises()

    expect(wrapper.find('[data-testid="invite-lottery-result"]').exists()).toBe(true)
    expect(wrapper.get('.lottery-cell--winner').text()).toContain('Prize 50')
    wrapper.unmount()
  })

  it('limits dense wheel labels while preserving all configured reward entries', async () => {
    api.getRechargeWheelStatus.mockResolvedValue({
      ...wheelStatus(),
      amounts: wheelAmounts(50),
      multipliers: wheelMultipliers(50),
    })
    const wrapper = mount(RechargeWheelView, { global })
    await flushPromises()

    expect(wrapper.get('.rw-stage').classes()).toContain('self-start')
    expect(wrapper.findAll('.rw-label--inner')).toHaveLength(8)
    expect(wrapper.findAll('.rw-label--outer')).toHaveLength(10)
    expect(wrapper.findAll('[data-testid="recharge-wheel-amount-item"]')).toHaveLength(50)
    expect(wrapper.findAll('[data-testid="recharge-wheel-multiplier-item"]')).toHaveLength(50)
    wrapper.unmount()
  })

  it('spaces a 20-tier milestone track evenly without dropping tiers', async () => {
    api.getInviteMilestoneStatus.mockResolvedValue({
      ...milestoneStatus(),
      invited_count: 0,
      qualified_invited_count: 0,
      tiers: milestoneTiers(20),
    })
    const wrapper = mount(InviteMilestoneView, { global })
    await flushPromises()

    const nodes = wrapper.findAll('.ms-node')
    expect(nodes).toHaveLength(20)
    expect((wrapper.get('.ms-track-space').element as HTMLElement).style.minWidth).toBe('1760px')
    expect((nodes[0]!.element as HTMLElement).style.left).toBe('5%')
    expect((nodes[19]!.element as HTMLElement).style.left).toBe('100%')
    wrapper.unmount()
  })

  it('preserves legacy threshold-proportional positions for a normal milestone set', async () => {
    api.getInviteMilestoneStatus.mockResolvedValue({
      ...milestoneStatus(),
      invited_count: 12,
      tiers: [1, 3, 10, 20].map((invites) => ({
        invites,
        reward: invites,
        reached: invites <= 12,
        recharge_reached: invites <= 12,
        blocked_by_recharge: false,
        claimed: false,
        claimable: false,
      })),
    })
    const wrapper = mount(InviteMilestoneView, { global })
    await flushPromises()

    const nodes = wrapper.findAll('.ms-node')
    expect(nodes.map((node) => (node.element as HTMLElement).style.left)).toEqual([
      '5%',
      '15%',
      '50%',
      '100%',
    ])
    expect((wrapper.get('.ms-fill').element as HTMLElement).style.width).toBe('60%')
    wrapper.unmount()
  })

  it('claims an invite lottery reward after the marquee animation', async () => {
    useAnimationTimers()
    api.claimInviteLottery.mockResolvedValue({
      ...lotteryStatus(),
      remaining_chances: 0,
      can_claim: false,
      prizes: [
        { name: 'Updated consolation', amount: 2 },
        { name: 'Updated winner', amount: 7 },
      ],
      prize: { name: 'Updated winner', amount: 7 },
    })
    const wrapper = mount(InviteLotteryView, { global })
    await flushPromises()

    expect(wrapper.get('.lottery-stage').classes()).not.toContain('self-start')
    await wrapper.get('[data-testid="invite-lottery-claim"]').trigger('click')
    await flushPromises()
    expect(appStore.fetchPublicSettings).toHaveBeenCalledWith(true)
    expect(api.claimInviteLottery).toHaveBeenCalledOnce()

    await vi.runAllTimersAsync()
    await flushPromises()
    expect(wrapper.find('[data-testid="invite-lottery-result"]').exists()).toBe(true)
    expect(wrapper.get('.lottery-cell--winner').text()).toContain('Updated winner')
    expect(showSuccess).toHaveBeenCalledOnce()
    wrapper.unmount()
  })

  it('claims a recharge wheel result after the spin and reveal animations', async () => {
    useAnimationTimers()
    api.claimRechargeWheel.mockResolvedValue({
      ...wheelStatus(),
      remaining_chances: 0,
      can_claim: false,
      amounts: [{ amount: 1 }, { amount: 5 }],
      multipliers: [{ multiplier: 2 }, { multiplier: 3 }, { multiplier: 4 }],
      result: { amount: 5, multiplier: 4, total: 20, amount_index: 1, multiplier_index: 2 },
    })
    const wrapper = mount(RechargeWheelView, { global })
    await flushPromises()

    expect(wrapper.get('.rw-stage').classes()).not.toContain('self-start')
    await wrapper.get('[data-testid="recharge-wheel-claim"]').trigger('click')
    await flushPromises()
    expect(appStore.fetchPublicSettings).toHaveBeenCalledWith(true)
    expect(api.claimRechargeWheel).toHaveBeenCalledOnce()
    expect((wrapper.get('.rw-wheel--inner').element as HTMLElement).style.transform).toBe(
      'rotate(2250deg)',
    )
    expect((wrapper.get('.rw-wheel--outer').element as HTMLElement).style.transform).toBe(
      'rotate(2220deg)',
    )

    await vi.runAllTimersAsync()
    await flushPromises()
    expect(wrapper.find('[data-testid="recharge-wheel-result"]').exists()).toBe(true)
    expect(showSuccess).toHaveBeenCalledOnce()
    wrapper.unmount()
  })

  it('passes the selected invite target and reveals the milestone reward', async () => {
    api.claimInviteMilestone.mockResolvedValue({
      ...milestoneStatus(),
      just_claimed_reward: 3,
      tiers: [
        {
          invites: 5,
          reward: 3,
          reached: true,
          recharge_reached: true,
          blocked_by_recharge: false,
          claimed: true,
          claimable: false,
        },
      ],
    })
    const wrapper = mount(InviteMilestoneView, { global })
    await flushPromises()

    await wrapper.get('[data-testid="invite-milestone-claim-5"]').trigger('click')
    await flushPromises()

    expect(appStore.fetchPublicSettings).toHaveBeenCalledWith(true)
    expect(api.claimInviteMilestone).toHaveBeenCalledWith(5)
    expect(wrapper.find('[data-testid="invite-milestone-result"]').exists()).toBe(true)
    expect(showSuccess).toHaveBeenCalledOnce()
    wrapper.unmount()
  })

  it('only applies lottery status when an idempotent response has no prize', async () => {
    api.claimInviteLottery.mockResolvedValue({
      ...lotteryStatus(),
      invited_count: 3,
      used_chances: 2,
      remaining_chances: 0,
      can_claim: false,
      prizes: [{ name: 'Updated pool prize', amount: 9 }],
    })
    const wrapper = mount(InviteLotteryView, { global })
    await flushPromises()

    await wrapper.get('[data-testid="invite-lottery-claim"]').trigger('click')
    await flushPromises()

    expect(api.claimInviteLottery).toHaveBeenCalledOnce()
    expect(wrapper.text()).toContain('Updated pool prize')
    expect(wrapper.find('[data-testid="invite-lottery-result"]').exists()).toBe(false)
    expect(showSuccess).not.toHaveBeenCalled()
    expect(showError).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('only applies wheel status when an idempotent response has no result', async () => {
    api.claimRechargeWheel.mockResolvedValue({
      ...wheelStatus(),
      used_chances: 2,
      remaining_chances: 0,
      can_claim: false,
      amounts: [{ amount: 4 }],
      multipliers: [{ multiplier: 5 }],
    })
    const wrapper = mount(RechargeWheelView, { global })
    await flushPromises()

    await wrapper.get('[data-testid="recharge-wheel-claim"]').trigger('click')
    await flushPromises()

    expect(api.claimRechargeWheel).toHaveBeenCalledOnce()
    expect(wrapper.get('.rw-label--inner').text()).toContain('$4')
    expect(wrapper.get('.rw-label--outer').text()).toContain('×5')
    expect(wrapper.find('[data-testid="recharge-wheel-result"]').exists()).toBe(false)
    expect(showSuccess).not.toHaveBeenCalled()
    expect(showError).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('only applies milestone status when an idempotent response has no claimed reward', async () => {
    api.claimInviteMilestone.mockResolvedValue({
      ...milestoneStatus(),
      tiers: [
        {
          ...milestoneStatus().tiers[0],
          claimed: true,
          claimable: false,
        },
      ],
    })
    const wrapper = mount(InviteMilestoneView, { global })
    await flushPromises()

    await wrapper.get('[data-testid="invite-milestone-claim-5"]').trigger('click')
    await flushPromises()

    expect(api.claimInviteMilestone).toHaveBeenCalledWith(5)
    expect(wrapper.get('[data-testid="invite-milestone-claim-5"]').attributes('disabled')).toBeDefined()
    expect(wrapper.find('[data-testid="invite-milestone-result"]').exists()).toBe(false)
    expect(showSuccess).not.toHaveBeenCalled()
    expect(showError).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('blocks the lottery POST when the forced public settings refresh fails', async () => {
    const wrapper = mount(InviteLotteryView, { global })
    await flushPromises()
    failNextPublicSettingsRefresh()

    await wrapper.get('[data-testid="invite-lottery-claim"]').trigger('click')
    await flushPromises()

    expect(appStore.fetchPublicSettings).toHaveBeenCalledWith(true)
    expect(api.claimInviteLottery).not.toHaveBeenCalled()
    expect(wrapper.find('[data-testid="invite-lottery-disabled"]').exists()).toBe(true)
    wrapper.unmount()
  })

  it('blocks the wheel POST when the forced public settings refresh fails', async () => {
    const wrapper = mount(RechargeWheelView, { global })
    await flushPromises()
    failNextPublicSettingsRefresh()

    await wrapper.get('[data-testid="recharge-wheel-claim"]').trigger('click')
    await flushPromises()

    expect(appStore.fetchPublicSettings).toHaveBeenCalledWith(true)
    expect(api.claimRechargeWheel).not.toHaveBeenCalled()
    expect(wrapper.find('[data-testid="recharge-wheel-disabled"]').exists()).toBe(true)
    wrapper.unmount()
  })

  it('blocks the milestone POST when the forced public settings refresh fails', async () => {
    const wrapper = mount(InviteMilestoneView, { global })
    await flushPromises()
    failNextPublicSettingsRefresh()

    await wrapper.get('[data-testid="invite-milestone-claim-5"]').trigger('click')
    await flushPromises()

    expect(appStore.fetchPublicSettings).toHaveBeenCalledWith(true)
    expect(api.claimInviteMilestone).not.toHaveBeenCalled()
    expect(wrapper.find('[data-testid="invite-milestone-disabled"]').exists()).toBe(true)
    wrapper.unmount()
  })

  it('rechecks the matching child switch before every reward POST', async () => {
    const lottery = mount(InviteLotteryView, { global })
    await flushPromises()
    appStore.cachedPublicSettings = {
      ...enabledSettings(),
      subnexus_invite_lottery_enabled: false,
    }
    await lottery.get('[data-testid="invite-lottery-claim"]').trigger('click')
    await flushPromises()
    expect(appStore.fetchPublicSettings).toHaveBeenCalledWith(true)
    expect(api.claimInviteLottery).not.toHaveBeenCalled()
    expect(lottery.find('[data-testid="invite-lottery-disabled"]').exists()).toBe(true)
    lottery.unmount()

    appStore.fetchPublicSettings.mockClear()
    appStore.cachedPublicSettings = enabledSettings()
    const wheel = mount(RechargeWheelView, { global })
    await flushPromises()
    appStore.cachedPublicSettings = {
      ...enabledSettings(),
      subnexus_recharge_wheel_enabled: false,
    }
    await wheel.get('[data-testid="recharge-wheel-claim"]').trigger('click')
    await flushPromises()
    expect(appStore.fetchPublicSettings).toHaveBeenCalledWith(true)
    expect(api.claimRechargeWheel).not.toHaveBeenCalled()
    expect(wheel.find('[data-testid="recharge-wheel-disabled"]').exists()).toBe(true)
    wheel.unmount()

    appStore.fetchPublicSettings.mockClear()
    appStore.cachedPublicSettings = enabledSettings()
    const milestone = mount(InviteMilestoneView, { global })
    await flushPromises()
    appStore.cachedPublicSettings = {
      ...enabledSettings(),
      subnexus_invite_milestone_enabled: false,
    }
    await milestone.get('[data-testid="invite-milestone-claim-5"]').trigger('click')
    await flushPromises()
    expect(appStore.fetchPublicSettings).toHaveBeenCalledWith(true)
    expect(api.claimInviteMilestone).not.toHaveBeenCalled()
    expect(milestone.find('[data-testid="invite-milestone-disabled"]').exists()).toBe(true)
    milestone.unmount()
  })

  it('settles and clears a lottery animation when unmounted', async () => {
    useAnimationTimers()
    api.claimInviteLottery.mockResolvedValue({
      ...lotteryStatus(),
      remaining_chances: 0,
      can_claim: false,
      prize: { name: 'Prize', amount: 1 },
    })
    const wrapper = mount(InviteLotteryView, { global })
    await flushPromises()

    await wrapper.get('[data-testid="invite-lottery-claim"]').trigger('click')
    await flushPromises()
    expect(vi.getTimerCount()).toBeGreaterThan(0)

    wrapper.unmount()
    expect(vi.getTimerCount()).toBe(0)
    await flushPromises()
    expect(showSuccess).not.toHaveBeenCalled()
  })

  it('cancels the wheel count-up frame when unmounted during the reveal', async () => {
    useAnimationTimers()
    api.claimRechargeWheel.mockResolvedValue({
      ...wheelStatus(),
      remaining_chances: 0,
      can_claim: false,
      result: { amount: 1, multiplier: 2, total: 2, amount_index: 0, multiplier_index: 0 },
    })
    const wrapper = mount(RechargeWheelView, { global })
    await flushPromises()

    await wrapper.get('[data-testid="recharge-wheel-claim"]').trigger('click')
    await flushPromises()
    await vi.advanceTimersByTimeAsync(5400)
    expect(wrapper.find('[data-testid="recharge-wheel-result"]').exists()).toBe(true)
    expect(showSuccess).not.toHaveBeenCalled()
    expect(vi.getTimerCount()).toBeGreaterThan(0)

    wrapper.unmount()
    expect(vi.getTimerCount()).toBe(0)
    await flushPromises()
    expect(showSuccess).not.toHaveBeenCalled()
  })
})
