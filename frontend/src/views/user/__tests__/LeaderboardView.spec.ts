import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import LeaderboardView from '@/views/user/LeaderboardView.vue'

const api = vi.hoisted(() => ({
  getLeaderboard: vi.fn(),
  getInviteLeaderboard: vi.fn(),
}))
const appStore = vi.hoisted(() => ({
  publicSettingsLoaded: true,
  cachedPublicSettings: { subnexus_leaderboard_enabled: true } as Record<string, boolean>,
  fetchPublicSettings: vi.fn(),
  showError: vi.fn(),
}))
const locale = vi.hoisted(() => ({ value: 'en' }))

vi.mock('@/api/leaderboard', () => api)
vi.mock('@/stores/app', () => ({ useAppStore: () => appStore }))
vi.mock('@/stores/auth', () => ({ useAuthStore: () => ({ user: { id: 7 } }) }))
vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ locale, t: (key: string) => key }),
  }
})

const global = {
  stubs: {
    AppLayout: { template: '<main><slot /></main>' },
    Icon: { template: '<i />' },
  },
}

function usageBoard(email = 'usage@example.com') {
  return {
    window: 'week',
    title: 'Usage',
    start_date: '2026-09-01',
    end_date: '2026-09-07',
    total_usage: 12,
    requests: 34,
    tokens: 10000,
    reward_top_n: 1,
    reward_value: 5,
    reward_amounts: [5],
    entries: [{ rank: 1, user_id: 7, email, usage: 12, requests: 34, tokens: 10000, rewarded: false }],
  }
}

function inviteBoard(email = 'invite@example.com') {
  return {
    window: 'month',
    title: 'Invites',
    start_date: '2026-09-01',
    end_date: '2026-09-30',
    total_invites: 8,
    entries: [{ rank: 1, user_id: 8, email, invite_count: 8 }],
  }
}

function deferred<T>() {
  let resolve!: (value: T) => void
  const promise = new Promise<T>((done) => { resolve = done })
  return { promise, resolve }
}

describe('LeaderboardView', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    appStore.publicSettingsLoaded = true
    appStore.cachedPublicSettings = { subnexus_leaderboard_enabled: true }
    locale.value = 'en'
    api.getLeaderboard.mockResolvedValue(usageBoard())
    api.getInviteLeaderboard.mockResolvedValue(inviteBoard())
  })

  it('does not query a board when the independent public flag is off', async () => {
    appStore.cachedPublicSettings = { subnexus_leaderboard_enabled: false }
    const wrapper = mount(LeaderboardView, { global })
    await flushPromises()

    expect(wrapper.find('[data-testid="leaderboard-disabled"]').exists()).toBe(true)
    expect(api.getLeaderboard).not.toHaveBeenCalled()
    expect(api.getInviteLeaderboard).not.toHaveBeenCalled()
  })

  it('ignores an older response after switching period and board type', async () => {
    const wrapper = mount(LeaderboardView, { global })
    await flushPromises()
    const oldUsage = deferred<ReturnType<typeof usageBoard>>()
    const currentInvites = deferred<ReturnType<typeof inviteBoard>>()
    api.getLeaderboard.mockReturnValueOnce(oldUsage.promise)
    api.getInviteLeaderboard.mockReturnValueOnce(currentInvites.promise)

    await wrapper.findAll('.seg-btn')[4].trigger('click')
    expect(wrapper.text()).toContain('common.loading')
    expect(wrapper.text()).not.toContain('usage@example.com')
    await wrapper.findAll('.seg-btn')[1].trigger('click')
    currentInvites.resolve(inviteBoard('current@example.com'))
    await flushPromises()
    expect(wrapper.text()).toContain('current@example.com')

    oldUsage.resolve(usageBoard('stale@example.com'))
    await flushPromises()
    expect(wrapper.text()).toContain('current@example.com')
    expect(wrapper.text()).not.toContain('stale@example.com')
  })

  it('keeps the legacy statistic captions and Chinese token precision', async () => {
    locale.value = 'zh-CN'
    api.getLeaderboard.mockResolvedValue({ ...usageBoard(), tokens: 38_600_000 })

    const wrapper = mount(LeaderboardView, { global })
    await flushPromises()

    const statisticCards = wrapper.findAll('.grid > .card')
    expect(statisticCards[0].findAll('p')[2].text()).toBe('leaderboard.period')
    expect(statisticCards[1].findAll('p')[2].text()).toBe('leaderboard.periodHint')
    expect(statisticCards[2].text()).toContain('3860.0万')
  })

  it('distinguishes a request failure from an empty board and retries it', async () => {
    api.getLeaderboard.mockRejectedValueOnce(new Error('network unavailable'))
    const wrapper = mount(LeaderboardView, { global })
    await flushPromises()

    const errorState = wrapper.find('[data-testid="leaderboard-load-error"]')
    expect(errorState.exists()).toBe(true)
    expect(errorState.text()).toContain('network unavailable')
    expect(errorState.text()).not.toContain('leaderboard.empty')
    expect(wrapper.find('[data-testid="leaderboard-my-rank"]').exists()).toBe(false)
    expect(appStore.showError).toHaveBeenCalledWith('network unavailable')

    await errorState.find('button').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="leaderboard-load-error"]').exists()).toBe(false)
    expect(wrapper.text()).toContain('usage@example.com')
  })
})
