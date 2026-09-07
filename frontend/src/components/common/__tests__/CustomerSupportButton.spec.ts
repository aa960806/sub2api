import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'

const state = vi.hoisted(() => ({
  route: { name: 'Dashboard' as string },
  appStore: {
    publicSettingsLoaded: true,
    cachedPublicSettings: {
      customer_support_enabled: true,
      customer_support_content: 'Contact us',
      home_content: '',
      compact_home_enabled: false,
    } as Record<string, boolean | string>,
  },
}))

vi.mock('@/stores', () => ({ useAppStore: () => state.appStore }))
vi.mock('vue-router', () => ({ useRoute: () => state.route }))
vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return { ...actual, useI18n: () => ({ t: (key: string) => key }) }
})

import CustomerSupportButton from '../CustomerSupportButton.vue'

const global = {
  stubs: {
    RainGlyph: { template: '<i data-testid="rain-support-glyph" />' },
    CustomerSupportModal: {
      props: ['visible'],
      template: '<div data-testid="support-modal-stub" :data-visible="String(visible)" />',
    },
  },
}

describe('CustomerSupportButton', () => {
  beforeEach(() => {
    state.route.name = 'Dashboard'
    state.appStore.publicSettingsLoaded = true
    state.appStore.cachedPublicSettings = {
      customer_support_enabled: true,
      customer_support_content: 'Contact us',
      home_content: '',
      compact_home_enabled: false,
    }
  })

  afterEach(() => {
    document.body.innerHTML = ''
  })

  it('fails closed until settings are loaded and support content is non-empty', () => {
    state.appStore.publicSettingsLoaded = false
    const wrapper = mount(CustomerSupportButton, { global })

    expect(document.body.querySelector('[data-testid="customer-support-button"]')).toBeNull()
    wrapper.unmount()
  })

  it('uses the legacy support button outside the Rain home and opens the modal', async () => {
    const wrapper = mount(CustomerSupportButton, { global })
    const button = document.body.querySelector('[data-testid="customer-support-button"]') as HTMLButtonElement

    expect(button).not.toBeNull()
    expect(button.classList.contains('support-btn')).toBe(true)
    button.click()
    await nextTick()
    expect(document.body.querySelector('[data-testid="support-modal-stub"]')?.getAttribute('data-visible')).toBe('true')
    wrapper.unmount()
  })

  it('keeps the dedicated Rain home button branch', () => {
    state.route.name = 'Home'
    const wrapper = mount(CustomerSupportButton, { global })
    const button = document.body.querySelector('[data-testid="customer-support-button"]') as HTMLButtonElement

    expect(button.classList.contains('rain-support-button')).toBe(true)
    expect(button.classList.contains('support-btn')).toBe(false)
    wrapper.unmount()
  })
})
