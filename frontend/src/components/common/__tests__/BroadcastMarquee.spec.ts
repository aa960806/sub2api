import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

const { listActive, appStore, authStore } = vi.hoisted(() => ({
  listActive: vi.fn(),
  appStore: {
    publicSettingsLoaded: false,
    cachedPublicSettings: null as null | Record<string, unknown>,
  },
  authStore: {
    isAuthenticated: true,
    user: { id: 7 },
  },
}))

vi.mock('@/api/marquee', () => ({ listActiveMarqueeBroadcasts: listActive }))
vi.mock('@/stores/app', () => ({ useAppStore: () => appStore }))
vi.mock('@/stores/auth', () => ({ useAuthStore: () => authStore }))
vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return { ...actual, useI18n: () => ({ t: (key: string) => key }) }
})

import BroadcastMarquee from '../BroadcastMarquee.vue'

function mountMarquee() {
  return mount(BroadcastMarquee, { global: { stubs: { Icon: true } } })
}

describe('BroadcastMarquee', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.clearAllMocks()
    window.localStorage.clear()
    authStore.isAuthenticated = true
    appStore.publicSettingsLoaded = false
    appStore.cachedPublicSettings = null
    listActive.mockResolvedValue({ enabled: true, items: [] })
  })

  it('does not request or schedule polling without an explicitly loaded true flag', async () => {
    const wrapper = mountMarquee()
    await vi.advanceTimersByTimeAsync(120_000)
    await flushPromises()
    expect(listActive).not.toHaveBeenCalled()
    expect(wrapper.find('[data-testid="broadcast-marquee"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('polls and displays only administrator-authored messages when explicitly enabled', async () => {
    appStore.publicSettingsLoaded = true
    appStore.cachedPublicSettings = { subnexus_marquee_enabled: true }
    listActive.mockResolvedValue({
      enabled: true,
      items: [
        { id: 1, title: 'Notice', content: 'Message', source: 'admin', enabled: true, priority: 1, created_at: '2026-01-01', updated_at: '2026-01-01' },
        { id: 2, title: 'Excluded', content: 'Spin reward', source: 'daily_spin', enabled: true, priority: 2, created_at: '2026-01-01', updated_at: '2026-01-01' },
      ],
    })
    const wrapper = mountMarquee()
    await flushPromises()
    expect(listActive).toHaveBeenCalledTimes(1)
    expect(wrapper.text()).toContain('Notice')
    expect(wrapper.text()).not.toContain('Spin reward')

    await vi.advanceTimersByTimeAsync(30_000)
    expect(listActive).toHaveBeenCalledTimes(2)
    wrapper.unmount()
  })

  it('stops polling and clears the local flag after a request failure', async () => {
    appStore.publicSettingsLoaded = true
    appStore.cachedPublicSettings = { subnexus_marquee_enabled: true }
    listActive.mockRejectedValueOnce(new Error('unavailable'))

    const wrapper = mountMarquee()
    await flushPromises()
    await vi.advanceTimersByTimeAsync(120_000)

    expect(listActive).toHaveBeenCalledTimes(1)
    expect(appStore.cachedPublicSettings?.subnexus_marquee_enabled).toBe(false)
    expect(wrapper.find('[data-testid="broadcast-marquee"]').exists()).toBe(false)
    wrapper.unmount()
  })
})
