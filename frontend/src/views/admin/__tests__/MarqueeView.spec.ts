import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

const { getConfig, listAdmin, updateConfig, appStore, showError, showSuccess } = vi.hoisted(() => ({
  getConfig: vi.fn(),
  listAdmin: vi.fn(),
  updateConfig: vi.fn(),
  appStore: { cachedPublicSettings: null as null | Record<string, unknown> },
  showError: vi.fn(),
  showSuccess: vi.fn(),
}))

vi.mock('@/api/marquee', () => ({
  getMarqueeConfig: getConfig,
  listAdminMarqueeBroadcasts: listAdmin,
  updateMarqueeConfig: updateConfig,
  createMarqueeBroadcast: vi.fn(),
  updateMarqueeBroadcast: vi.fn(),
  deleteMarqueeBroadcast: vi.fn(),
}))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({
  get cachedPublicSettings() { return appStore.cachedPublicSettings },
  set cachedPublicSettings(value) { appStore.cachedPublicSettings = value },
  showError,
  showSuccess,
}) }))
vi.mock('@/utils/apiError', () => ({ extractApiErrorMessage: (_error: unknown, fallback: string) => fallback }))
vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return { ...actual, useI18n: () => ({ t: (key: string) => key }) }
})

import MarqueeView from '../MarqueeView.vue'

function mountView() {
  return mount(MarqueeView, {
    global: {
      stubs: {
        AppLayout: { template: '<div><slot /></div>' },
        Icon: true,
        Toggle: true,
        ConfirmDialog: true,
      },
    },
  })
}

describe('admin marquee view', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    appStore.cachedPublicSettings = { subnexus_marquee_enabled: false }
    getConfig.mockResolvedValue({ enabled: false })
    listAdmin.mockResolvedValue([])
  })

  it('does not query broadcasts while the confirmed switch is off', async () => {
    const wrapper = mountView()
    await flushPromises()
    expect(wrapper.find('[data-testid="marquee-disabled"]').exists()).toBe(true)
    expect(listAdmin).not.toHaveBeenCalled()
  })

  it('fails closed and does not query broadcasts when config loading fails', async () => {
    getConfig.mockRejectedValueOnce(new Error('unavailable'))
    const wrapper = mountView()
    await flushPromises()
    expect(wrapper.find('[data-testid="marquee-config-error"]').exists()).toBe(true)
    expect(listAdmin).not.toHaveBeenCalled()
    expect(appStore.cachedPublicSettings?.subnexus_marquee_enabled).toBe(false)
    expect(showError).toHaveBeenCalledWith('admin.marquee.loadFailed')
  })

  it('loads administrator broadcasts only after the backend confirms enabled', async () => {
    getConfig.mockResolvedValueOnce({ enabled: true })
    const wrapper = mountView()
    await flushPromises()
    expect(listAdmin).toHaveBeenCalledTimes(1)
    expect(wrapper.find('[data-testid="marquee-form"]').exists()).toBe(true)
  })
})
