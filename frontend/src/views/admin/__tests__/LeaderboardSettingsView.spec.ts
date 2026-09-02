import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import LeaderboardSettingsView from '../LeaderboardSettingsView.vue'

const {
  getLeaderboardConfig,
  updateLeaderboardConfig,
  showError,
  showSuccess,
  appStore,
} = vi.hoisted(() => ({
  getLeaderboardConfig: vi.fn(),
  updateLeaderboardConfig: vi.fn(),
  showError: vi.fn(),
  showSuccess: vi.fn(),
  appStore: {
    cachedPublicSettings: null as Record<string, unknown> | null,
  },
}))

vi.mock('@/api/leaderboard', () => ({
  getLeaderboardConfig,
  updateLeaderboardConfig,
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    ...appStore,
    get cachedPublicSettings() {
      return appStore.cachedPublicSettings
    },
    set cachedPublicSettings(value) {
      appStore.cachedPublicSettings = value
    },
    showError,
    showSuccess,
  }),
}))

vi.mock('@/utils/apiError', () => ({
  extractApiErrorMessage: (_error: unknown, fallback: string) => fallback,
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key }),
  }
})

const config = {
  enabled: true,
  weekly_enabled: false,
  weekly_top_n: 3,
  weekly_reward: 1,
  weekly_rewards: [1, 1, 1],
  monthly_enabled: false,
  monthly_top_n: 3,
  monthly_reward: 5,
  monthly_rewards: [5, 5, 5],
}

function mountView() {
  return mount(LeaderboardSettingsView, {
    global: {
      stubs: {
        AppLayout: { template: '<div><slot /></div>' },
        Icon: true,
        Toggle: true,
      },
    },
  })
}

describe('admin leaderboard settings', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    appStore.cachedPublicSettings = { subnexus_leaderboard_enabled: true }
    getLeaderboardConfig.mockResolvedValue({ ...config })
    updateLeaderboardConfig.mockResolvedValue({ ...config })
  })

  it('fails closed and does not expose a default form after a load error', async () => {
    getLeaderboardConfig.mockRejectedValueOnce(new Error('unavailable'))
    const wrapper = mountView()

    await flushPromises()

    expect(wrapper.find('[data-testid="leaderboard-load-error"]').exists()).toBe(true)
    expect(wrapper.find('form').exists()).toBe(false)
    expect(appStore.cachedPublicSettings?.subnexus_leaderboard_enabled).toBe(false)
    expect(showError).toHaveBeenCalledWith('admin.leaderboard.loadFailed')
    expect(updateLeaderboardConfig).not.toHaveBeenCalled()
  })

  it('retries a failed read before allowing configuration changes', async () => {
    getLeaderboardConfig
      .mockRejectedValueOnce(new Error('unavailable'))
      .mockResolvedValueOnce({ ...config })
    const wrapper = mountView()
    await flushPromises()

    await wrapper.get('[data-testid="leaderboard-retry"]').trigger('click')
    await flushPromises()

    expect(getLeaderboardConfig).toHaveBeenCalledTimes(2)
    expect(wrapper.find('[data-testid="leaderboard-load-error"]').exists()).toBe(false)
    expect(wrapper.find('form').exists()).toBe(true)
    expect(appStore.cachedPublicSettings?.subnexus_leaderboard_enabled).toBe(true)
  })

  it('preserves the last confirmed public flag when saving fails', async () => {
    updateLeaderboardConfig.mockRejectedValueOnce(new Error('save failed'))
    const wrapper = mountView()
    await flushPromises()

    await wrapper.get('form').trigger('submit.prevent')
    await flushPromises()

    expect(updateLeaderboardConfig).toHaveBeenCalledTimes(1)
    expect(appStore.cachedPublicSettings?.subnexus_leaderboard_enabled).toBe(true)
    expect(showError).toHaveBeenCalledWith('admin.leaderboard.saveFailed')
  })
})
