import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import en from '@/i18n/locales/en'
import zh from '@/i18n/locales/zh'

const { appStore } = vi.hoisted(() => ({
  appStore: {
    cachedPublicSettings: {
      site_name: 'Sub2API',
      customer_support_content: '',
    } as Record<string, string>,
  },
}))

vi.mock('@/stores', () => ({ useAppStore: () => appStore }))
vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return { ...actual, useI18n: () => ({ t: (key: string) => key }) }
})

import CustomerSupportModal from '../CustomerSupportModal.vue'

describe('CustomerSupportModal Markdown sanitization', () => {
  beforeEach(() => {
    document.body.innerHTML = ''
    appStore.cachedPublicSettings.customer_support_content = ''
    document.body.style.overflow = ''
  })

  afterEach(() => {
    document.body.innerHTML = ''
    document.body.style.overflow = ''
  })

  it('removes executable markup and protects blank-target links', () => {
    appStore.cachedPublicSettings.customer_support_content = [
      '<script>alert(1)</script>',
      '<a href="javascript:alert(2)" target="_blank">bad</a>',
      '<a href="https://support.example" target="_blank" rel="nofollow">safe</a>',
      '<a href="data:image/png;base64,AAAA">unsafe-data-link</a>',
      '<img src="data:image/png;base64,AAAA" alt="qr">',
      '<img src="data:image/svg+xml;base64,PHN2Zy8+" alt="unsafe-svg">',
    ].join('\n')

    const wrapper = mount(CustomerSupportModal, {
      props: { visible: true },
      attachTo: document.body,
      global: { stubs: { Icon: true } },
    })

    const content = document.body.querySelector('[data-testid="customer-support-content"]') as HTMLElement
    expect(content).not.toBeNull()
    const html = content.innerHTML
    expect(html).not.toContain('<script')
    expect(html).not.toContain('javascript:')
    const safeLink = content.querySelector('a[href="https://support.example"]') as HTMLAnchorElement
    expect(safeLink).not.toBeNull()
    expect(safeLink.target).toBe('_blank')
    expect(safeLink.rel).toContain('noopener')
    expect(safeLink.rel).toContain('noreferrer')
    expect((content.querySelector('img') as HTMLImageElement).src).toMatch(/^data:image\/png;/)
    expect(content.innerHTML).not.toContain('data:image/svg+xml')
    expect(content.querySelector('a[href^="data:"]')).toBeNull()
    wrapper.unmount()
  })

  it('locks scrolling while visible and restores the prior body style', async () => {
    document.body.style.overflow = 'clip'
    appStore.cachedPublicSettings.customer_support_content = 'Support content'
    const wrapper = mount(CustomerSupportModal, {
      props: { visible: true },
      attachTo: document.body,
    })

    expect(document.body.style.overflow).toBe('hidden')
    await wrapper.setProps({ visible: false })
    expect(document.body.style.overflow).toBe('clip')
    wrapper.unmount()
  })

  it('closes from the overlay and Escape key', async () => {
    appStore.cachedPublicSettings.customer_support_content = 'Support content'
    const overlayWrapper = mount(CustomerSupportModal, {
      props: { visible: true },
      attachTo: document.body,
    })
    const overlay = document.body.querySelector('.modal-overlay') as HTMLElement

    overlay.click()
    await nextTick()
    expect(overlayWrapper.emitted('update:visible')?.at(-1)).toEqual([false])
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    expect(overlayWrapper.emitted('update:visible')?.at(-1)).toEqual([false])
    overlayWrapper.unmount()
  })

  it('renders localized support copy and defines both locale values', () => {
    appStore.cachedPublicSettings.customer_support_content = '<script>alert(1)</script>'
    const wrapper = mount(CustomerSupportModal, {
      props: { visible: true },
      attachTo: document.body,
    })

    expect(document.body.querySelector('.modal-subtitle')?.textContent).toBe(
      'common.customerSupportSubtitle',
    )
    expect(document.body.querySelector('.empty-state')?.textContent?.trim()).toBe(
      'common.customerSupportEmpty',
    )
    expect(zh.common.customerSupportSubtitle).toBe('我们随时为您提供帮助')
    expect(zh.common.customerSupportEmpty).toBe('暂无客服信息')
    expect(en.common.customerSupportSubtitle).toBe('We are here to help')
    expect(en.common.customerSupportEmpty).toBe('No support information available')
    wrapper.unmount()
  })
})
