import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

const { getCheckInConfig, updateCheckInConfig, appStore, showError, showSuccess } = vi.hoisted(() => ({
  getCheckInConfig: vi.fn(),
  updateCheckInConfig: vi.fn(),
  appStore: { cachedPublicSettings: null as Record<string, unknown> | null },
  showError: vi.fn(),
  showSuccess: vi.fn(),
}))

vi.mock('@/api/checkin', () => ({ getCheckInConfig, updateCheckInConfig }))
vi.mock('@/stores', () => ({ useAppStore: () => ({
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

import CheckInSettingsView from '../CheckInSettingsView.vue'

const config = {
  enabled: false,
  checkin_ip_limit: true,
  checkin_min: 0.01,
  checkin_max: 0.1,
  checkin_cycle_mode: 'reset' as const,
  checkin_milestone_days: 7,
  checkin_milestone_min: 0.1,
  checkin_milestone_max: 0.5,
  checkin_paid_mode: 'off' as const,
  checkin_free_max_count: 0,
  checkin_free_max_amount: 0,
  checkin_over_limit_action: 'prompt' as const,
}

function mountView() {
  return mount(CheckInSettingsView, {
    global: {
      stubs: {
        AppLayout: { template: '<div><slot /></div>' },
        Icon: true,
        Toggle: true,
        TotpStepUpDialog: true,
      },
    },
  })
}

describe('admin check-in settings', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    appStore.cachedPublicSettings = { subnexus_checkin_enabled: true }
    getCheckInConfig.mockResolvedValue({ ...config })
    updateCheckInConfig.mockResolvedValue({ ...config })
  })

  it('keeps the admin configuration route available while check-in is disabled', async () => {
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.find('[data-testid="admin-checkin-page"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="checkin-enabled"]').exists()).toBe(true)
    expect(appStore.cachedPublicSettings?.subnexus_checkin_enabled).toBe(false)
  })

  it('fails closed when the configuration read fails', async () => {
    getCheckInConfig.mockRejectedValueOnce(new Error('unavailable'))
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.find('[data-testid="checkin-load-error"]').exists()).toBe(true)
    expect(wrapper.find('form').exists()).toBe(false)
    expect(appStore.cachedPublicSettings?.subnexus_checkin_enabled).toBe(false)
    expect(showError).toHaveBeenCalledWith('admin.checkin.loadFailed')
  })

  it('persists the loaded policy through the existing admin API', async () => {
    const wrapper = mountView()
    await flushPromises()
    await wrapper.get('form').trigger('submit.prevent')
    await flushPromises()

    expect(updateCheckInConfig).toHaveBeenCalledWith(config)
    expect(showSuccess).toHaveBeenCalledWith('admin.checkin.saved')
  })
})
