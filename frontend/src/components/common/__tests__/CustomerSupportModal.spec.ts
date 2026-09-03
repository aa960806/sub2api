import { beforeEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'

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
    appStore.cachedPublicSettings.customer_support_content = ''
  })

  it('removes executable markup and protects blank-target links', () => {
    appStore.cachedPublicSettings.customer_support_content = [
      '<script>alert(1)</script>',
      '<a href="javascript:alert(2)" target="_blank">bad</a>',
      '<a href="https://support.example" target="_blank" rel="nofollow">safe</a>',
      '<img src="data:image/png;base64,AAAA" alt="qr">',
    ].join('\n')

    const wrapper = mount(CustomerSupportModal, {
      props: { visible: true },
      attachTo: document.body,
      global: { stubs: { Icon: true } },
    })

    const content = document.body.querySelector('.markdown-body') as HTMLElement
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
    wrapper.unmount()
  })
})
