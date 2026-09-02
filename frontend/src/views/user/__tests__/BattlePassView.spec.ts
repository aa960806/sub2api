import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { reactive } from 'vue'
import BattlePassView from '../BattlePassView.vue'

const api = vi.hoisted(() => ({
  claimAllBattlePassRewards: vi.fn(),
  claimBattlePassReward: vi.fn(),
  equipBattlePassCosmetic: vi.fn(),
  getBattlePassCosmetics: vi.fn(),
  getBattlePassCurrent: vi.fn(),
  getBattlePassHistory: vi.fn(),
  getBattlePassRewards: vi.fn(),
  getBattlePassTasks: vi.fn(),
  purchaseBattlePass: vi.fn(),
}))
const showSuccess = vi.hoisted(() => vi.fn())
const showError = vi.hoisted(() => vi.fn())
const routerReplace = vi.hoisted(() => vi.fn())
const appStoreState = vi.hoisted(() => ({
  showSuccess,
  showError,
  publicSettingsLoaded: true,
  cachedPublicSettings: {
    subnexus_activity_center_enabled: false,
    battle_pass_enabled: true,
  } as null | {
    subnexus_activity_center_enabled?: boolean
    battle_pass_enabled?: boolean
  },
  fetchPublicSettings: vi.fn(),
}))
const appStore = reactive(appStoreState)

vi.mock('@/api/battlePass', () => ({
  ...api,
  battlePassLevelProgress: () => 35,
}))
vi.mock('@/stores', () => ({ useAppStore: () => appStore }))
vi.mock('vue-router', () => ({ useRouter: () => ({ replace: routerReplace }) }))
vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return { ...actual, useI18n: () => ({ t: (key: string) => key }) }
})

const rewards = [
  { id: 11, level: 1, track: 'free', reward_type: 'balance', payload: { amount: 2 }, status: 'claimable' },
  { id: 12, level: 1, track: 'premium', reward_type: 'concurrency', payload: { amount: 1 }, status: 'premium_locked' },
  { id: 21, level: 2, track: 'free', reward_type: 'badge', payload: { name: '先锋徽章' }, status: 'locked' },
  { id: 22, level: 2, track: 'premium', reward_type: 'title', payload: { name: '远征者' }, status: 'locked' },
] as const

function mountView() {
  return mount(BattlePassView, {
    global: {
      stubs: {
        AppLayout: { template: '<main><slot /></main>' },
        Icon: { template: '<i />' },
      },
    },
  })
}

function deferred<T>() {
  let resolve!: (value: T) => void
  const promise = new Promise<T>((resolvePromise) => {
    resolve = resolvePromise
  })
  return { promise, resolve }
}

describe('BattlePassView manual rewards', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    appStore.publicSettingsLoaded = true
    appStore.cachedPublicSettings = {
      subnexus_activity_center_enabled: false,
      battle_pass_enabled: true,
    }
    appStore.fetchPublicSettings.mockResolvedValue(appStore.cachedPublicSettings)
    api.getBattlePassCurrent.mockResolvedValue({
      user_side_enabled: true,
      season: { id: 7, name: '第一赛季', description: '', runtime_status: 'active', start_at: '2026-08-01T00:00:00Z', end_at: '2026-10-01T00:00:00Z', premium_price: 10 },
      progress: { exp: 35, level: 1, level_start_exp: 0, next_level_exp: 100, premium_unlocked: false },
    })
    api.getBattlePassTasks.mockResolvedValue([])
    api.getBattlePassRewards.mockResolvedValue(rewards.map((item) => ({ ...item })))
    api.getBattlePassHistory.mockResolvedValue({ experience: [], purchases: [], rewards: [] })
    api.getBattlePassCosmetics.mockResolvedValue([])
  })

  it('shows the full free and premium tracks with distinct locked states', async () => {
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.text()).toContain('赛季奖励总览')
    expect(wrapper.text()).toContain('高级未解锁')
    expect(wrapper.text()).toContain('未达成')
    expect(wrapper.text()).toContain('先锋徽章')
    expect(wrapper.text()).toContain('远征者')
    expect(wrapper.get('button[aria-label*="等级 1 高级奖励"]').attributes('disabled')).toBeDefined()
    expect(wrapper.get('button[aria-label*="等级 2 免费奖励"]').attributes('disabled')).toBeDefined()
    expect(wrapper.get('.bp-claim-all').attributes('disabled')).toBeUndefined()
  })

  it('fails closed when the server does not explicitly enable the user side', async () => {
    api.getBattlePassCurrent.mockResolvedValue({
      season: null,
      syncing: false,
    })

    mountView()
    await flushPromises()

    expect(routerReplace).toHaveBeenCalledWith('/dashboard')
    expect(api.getBattlePassTasks).not.toHaveBeenCalled()
    expect(api.getBattlePassRewards).not.toHaveBeenCalled()
    expect(api.getBattlePassHistory).not.toHaveBeenCalled()
    expect(api.getBattlePassCosmetics).not.toHaveBeenCalled()
  })

  it('does not require an activity-center card when its own gate is enabled', async () => {
    appStore.cachedPublicSettings = {
      subnexus_activity_center_enabled: false,
      battle_pass_enabled: true,
    }

    mountView()
    await flushPromises()

    expect(api.getBattlePassCurrent).toHaveBeenCalledOnce()
    expect(routerReplace).not.toHaveBeenCalled()
  })

  it('shows the empty state without loading season data when access is enabled but no season exists', async () => {
    api.getBattlePassCurrent.mockResolvedValue({
      season: null,
      syncing: false,
      user_side_enabled: true,
    })

    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.text()).toContain('battlePass.emptySeason')
    expect(routerReplace).not.toHaveBeenCalled()
    expect(api.getBattlePassTasks).not.toHaveBeenCalled()
    expect(api.getBattlePassRewards).not.toHaveBeenCalled()
  })

  it('does not expose battle-pass data when the gate request fails', async () => {
    api.getBattlePassCurrent.mockRejectedValue({ code: 'BATTLE_PASS_DISABLED', message: 'settings unavailable' })

    mountView()
    await flushPromises()

    expect(showError).toHaveBeenCalledWith('settings unavailable')
    expect(routerReplace).toHaveBeenCalledWith('/dashboard')
    expect(api.getBattlePassTasks).not.toHaveBeenCalled()
  })

  it('does not query the battle-pass endpoint while its public switch is off', async () => {
    appStore.cachedPublicSettings = {
      subnexus_activity_center_enabled: true,
      battle_pass_enabled: false,
    }

    mountView()
    await flushPromises()

    expect(api.getBattlePassCurrent).not.toHaveBeenCalled()
    expect(routerReplace).toHaveBeenCalledWith('/dashboard')
  })

  it('claims one eligible reward and renders the granted state', async () => {
    api.claimBattlePassReward.mockResolvedValue({
      claimed_count: 1,
      rewards: rewards.map((item) => item.id === 11 ? { ...item, status: 'granted' } : { ...item }),
    })
    const wrapper = mountView()
    await flushPromises()

    await wrapper.get('button[aria-label*="等级 1 免费奖励"]').trigger('click')
    await flushPromises()

    expect(api.claimBattlePassReward).toHaveBeenCalledWith(11)
    expect(wrapper.text()).toContain('已领取')
    expect(showSuccess).toHaveBeenCalledWith('奖励已领取')
  })

  it('claims every eligible reward from the footer action', async () => {
    api.claimAllBattlePassRewards.mockResolvedValue({
      claimed_count: 1,
      rewards: rewards.map((item) => item.id === 11 ? { ...item, status: 'granted' } : { ...item }),
    })
    const wrapper = mountView()
    await flushPromises()

    await wrapper.get('.bp-claim-all').trigger('click')
    await flushPromises()

    expect(api.claimAllBattlePassRewards).toHaveBeenCalledOnce()
    expect(showSuccess).toHaveBeenCalledWith('已领取 1 项奖励')
    expect(wrapper.get('.bp-claim-all').attributes('disabled')).toBeDefined()
  })

  it('shows the upcoming season instead of presenting it as active', async () => {
    const start = new Date(Date.now() + 2 * 3600_000)
    const end = new Date(start.getTime() + 30 * 86400_000)
    api.getBattlePassCurrent.mockResolvedValue({
      user_side_enabled: true,
      season: { id: 8, name: '第二赛季', description: '', runtime_status: 'scheduled', start_at: start.toISOString(), end_at: end.toISOString(), premium_price: 10 },
      progress: null,
    })
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.text()).toContain('第二赛季')
    expect(wrapper.text()).toContain('即将开始')
    expect(wrapper.text()).toContain('距开始 2 小时')
    expect(wrapper.text()).toContain('赛季开始后完成任务')
    expect(wrapper.get('.bp-claim-all').attributes('disabled')).toBeDefined()
  })

  it('explains and labels wearable titles and badges', async () => {
    api.getBattlePassCosmetics.mockResolvedValue([
      { id: 31, kind: 'title', code: 'pioneer', name: '远征者', color_token: '', asset_key: '', equipped: true, granted_at: '2026-09-01T00:00:00Z' },
      { id: 32, kind: 'badge', code: 'gold', name: '黄金徽章', color_token: '', asset_key: '', equipped: false, granted_at: '2026-09-01T00:00:00Z' },
    ])
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.text()).toContain('称号和徽章可分别佩戴一项')
    expect(wrapper.text()).toContain('称号远征者 · 已佩戴')
    expect(wrapper.text()).toContain('徽章黄金徽章')
  })

  it('blocks reward mutations after the public switch is disabled on an open page', async () => {
    api.purchaseBattlePass.mockResolvedValue(undefined)
    api.claimBattlePassReward.mockResolvedValue({ claimed_count: 1, rewards: rewards.map((item) => ({ ...item })) })
    api.claimAllBattlePassRewards.mockResolvedValue({ claimed_count: 1, rewards: rewards.map((item) => ({ ...item })) })
    api.getBattlePassCosmetics.mockResolvedValue([
      { id: 31, kind: 'title', code: 'pioneer', name: '远征者', color_token: '', asset_key: '', equipped: false, granted_at: '2026-09-01T00:00:00Z' },
    ])
    const wrapper = mountView()
    await flushPromises()

    appStore.cachedPublicSettings = {
      subnexus_activity_center_enabled: false,
      battle_pass_enabled: false,
    }

    // Trigger handlers before the queued watcher removes the page data. Each
    // handler must re-check the independent gate immediately before writing.
    void wrapper.get('button[aria-label*="等级 1 免费奖励"]').trigger('click')
    void wrapper.get('.bp-claim-all').trigger('click')
    void wrapper.get('button.btn-primary.mt-4').trigger('click')
    void wrapper.get('.bp-cosmetic').trigger('click')
    await flushPromises()

    expect(api.purchaseBattlePass).not.toHaveBeenCalled()
    expect(api.claimBattlePassReward).not.toHaveBeenCalled()
    expect(api.claimAllBattlePassRewards).not.toHaveBeenCalled()
    expect(api.equipBattlePassCosmetic).not.toHaveBeenCalled()
    expect(routerReplace).toHaveBeenCalledWith('/dashboard')
  })

  it('does not fetch history when the switch closes during a claim request', async () => {
    const pendingClaim = deferred<{
      claimed_count: number
      rewards: Array<(typeof rewards)[number]>
    }>()
    api.claimBattlePassReward.mockReturnValue(pendingClaim.promise)
    const wrapper = mountView()
    await flushPromises()
    api.getBattlePassHistory.mockClear()

    await wrapper.get('button[aria-label*="等级 1 免费奖励"]').trigger('click')
    expect(api.claimBattlePassReward).toHaveBeenCalledWith(11)

    appStore.cachedPublicSettings = { battle_pass_enabled: false }
    pendingClaim.resolve({
      claimed_count: 1,
      rewards: rewards.map((item) => ({ ...item })),
    })
    await flushPromises()

    expect(api.getBattlePassHistory).not.toHaveBeenCalled()
    expect(showSuccess).not.toHaveBeenCalled()
    expect(routerReplace).toHaveBeenCalledWith('/dashboard')
  })

  it('does not refetch cosmetics when the switch closes during an equip request', async () => {
    const pendingEquip = deferred<void>()
    api.getBattlePassCosmetics.mockResolvedValue([
      { id: 31, kind: 'title', code: 'pioneer', name: '远征者', color_token: '', asset_key: '', equipped: false, granted_at: '2026-09-01T00:00:00Z' },
    ])
    api.equipBattlePassCosmetic.mockReturnValue(pendingEquip.promise)
    const wrapper = mountView()
    await flushPromises()
    api.getBattlePassCosmetics.mockClear()

    await wrapper.get('.bp-cosmetic').trigger('click')
    expect(api.equipBattlePassCosmetic).toHaveBeenCalledWith(31)

    appStore.cachedPublicSettings = { battle_pass_enabled: false }
    pendingEquip.resolve()
    await flushPromises()

    expect(api.getBattlePassCosmetics).not.toHaveBeenCalled()
    expect(routerReplace).toHaveBeenCalledWith('/dashboard')
  })
})
