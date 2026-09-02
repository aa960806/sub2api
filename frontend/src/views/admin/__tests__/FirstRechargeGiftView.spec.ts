import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import FirstRechargeGiftView from '../FirstRechargeGiftView.vue'

const { getConfig, updateConfig, showError, showSuccess } = vi.hoisted(() => ({
  getConfig: vi.fn(),
  updateConfig: vi.fn(),
  showError: vi.fn(),
  showSuccess: vi.fn(),
}))

vi.mock('@/api/admin/payment', () => {
  const api = {
    getFirstRechargeGiftConfig: getConfig,
    updateFirstRechargeGiftConfig: updateConfig,
  }
  return { adminPaymentAPI: api, default: api }
})

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showError, showSuccess }),
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

const disabledConfig = {
  enabled: false,
  price: 9.9,
  credited_amount: 12,
  ratio: 1.2121,
}

function mountView() {
  return mount(FirstRechargeGiftView, {
    global: {
      stubs: {
        AppLayout: { template: '<main><slot /></main>' },
        Icon: true,
      },
    },
  })
}

describe('admin first recharge gift settings', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    getConfig.mockResolvedValue({ data: { ...disabledConfig } })
    updateConfig.mockImplementation(async (config) => ({ data: config }))
  })

  it('loads as disabled and treats non-boolean enable values as off', async () => {
    getConfig.mockResolvedValueOnce({ data: { ...disabledConfig, enabled: 'true' } })
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.get('[data-testid="first-recharge-enabled"]').attributes('aria-checked')).toBe('false')
    expect(wrapper.find('form').exists()).toBe(true)
  })

  it('fails closed without exposing a submit form and supports retry', async () => {
    getConfig
      .mockRejectedValueOnce(new Error('unavailable'))
      .mockResolvedValueOnce({ data: { ...disabledConfig } })
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.find('[data-testid="first-recharge-load-error"]').exists()).toBe(true)
    expect(wrapper.find('form').exists()).toBe(false)
    expect(updateConfig).not.toHaveBeenCalled()

    await wrapper.get('[data-testid="first-recharge-retry"]').trigger('click')
    await flushPromises()

    expect(getConfig).toHaveBeenCalledTimes(2)
    expect(wrapper.find('form').exists()).toBe(true)
  })

  it('rounds amounts, calculates the ratio, and saves the independent switch', async () => {
    const wrapper = mountView()
    await flushPromises()

    await wrapper.get('[data-testid="first-recharge-enabled"]').trigger('click')
    await wrapper.get('[data-testid="first-recharge-price"]').setValue('10.235')
    await wrapper.get('[data-testid="first-recharge-credit"]').setValue('15.678')
    await wrapper.get('form').trigger('submit.prevent')
    await flushPromises()

    expect(updateConfig).toHaveBeenCalledWith({
      enabled: true,
      price: 10.24,
      credited_amount: 15.68,
      ratio: 1.5313,
    })
    expect(showSuccess).toHaveBeenCalledWith('admin.firstRechargeGift.saved')
  })

  it('restores the last confirmed configuration when saving fails', async () => {
    updateConfig.mockRejectedValueOnce(new Error('save failed'))
    const wrapper = mountView()
    await flushPromises()

    await wrapper.get('[data-testid="first-recharge-enabled"]').trigger('click')
    await wrapper.get('form').trigger('submit.prevent')
    await flushPromises()

    expect(wrapper.get('[data-testid="first-recharge-enabled"]').attributes('aria-checked')).toBe('false')
    expect(showError).toHaveBeenCalledWith('admin.firstRechargeGift.saveFailed')
  })

  it('does not call the update endpoint for an invalid payment amount', async () => {
    const wrapper = mountView()
    await flushPromises()

    await wrapper.get('[data-testid="first-recharge-price"]').setValue('0')
    await wrapper.get('form').trigger('submit.prevent')
    await flushPromises()

    expect(updateConfig).not.toHaveBeenCalled()
    expect(showError).toHaveBeenCalledWith('admin.firstRechargeGift.invalidPrice')
  })
})
