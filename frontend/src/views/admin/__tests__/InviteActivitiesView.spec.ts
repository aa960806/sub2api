import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

const { getConfig, updateConfig, appStore, showError, showSuccess } = vi.hoisted(() => ({
  getConfig: vi.fn(),
  updateConfig: vi.fn(),
  appStore: { cachedPublicSettings: null as Record<string, unknown> | null },
  showError: vi.fn(),
  showSuccess: vi.fn(),
}))

vi.mock('@/api/inviteActivities', () => ({
  getInviteActivitiesConfig: getConfig,
  updateInviteActivitiesConfig: updateConfig,
}))
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

import InviteActivitiesView from '../InviteActivitiesView.vue'

function mountView() {
  return mount(InviteActivitiesView, {
    global: {
      stubs: {
        AppLayout: { template: '<div><slot /></div>' },
        Icon: true,
        Toggle: true,
      },
    },
  })
}

describe('admin invite activities view', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    appStore.cachedPublicSettings = { subnexus_invite_activities_enabled: false }
    updateConfig.mockResolvedValue({ enabled: false })
  })

  it('fills omitted policy arrays when the closed rollout config returns null fields', async () => {
    getConfig.mockResolvedValue({
      enabled: false,
      invite_lottery_enabled: false,
      invite_lottery_prizes: null,
      invite_lottery_recharge_limit_enabled: false,
      invite_lottery_recharge_threshold: 10,
      recharge_wheel_enabled: false,
      recharge_wheel_threshold: 10,
      recharge_wheel_amounts: null,
      recharge_wheel_multipliers: null,
      invite_milestone_enabled: false,
      invite_milestone_tiers: null,
      invite_milestone_recharge_limit_enabled: false,
      invite_milestone_recharge_threshold: 10,
    })

    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.find('[data-testid="invite-activities-load-error"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="invite-activities-save"]').exists()).toBe(true)
    expect(wrapper.findAll('.config-row')).toHaveLength(15)
    expect(showError).not.toHaveBeenCalled()
  })
})
