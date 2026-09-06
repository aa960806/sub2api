import { describe, expect, it, vi } from 'vitest'
import { mount, RouterLinkStub } from '@vue/test-utils'
import RainGatewayHome from '../RainGatewayHome.vue'

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key }),
  }
})

const baseProps = {
  siteName: 'Configured Gateway',
  siteLogo: '/configured-logo.svg',
  siteSubtitle: 'Configured subtitle',
  docUrl: 'https://docs.example.test',
  showModelPlazaEntry: true,
  isAuthenticated: false,
  dashboardPath: '/dashboard',
  userInitial: '',
  isDark: true,
  currentYear: 2026,
}

function mountHome(overrides: Partial<typeof baseProps> = {}) {
  return mount(RainGatewayHome, {
    props: { ...baseProps, ...overrides },
    global: {
      stubs: {
        RouterLink: RouterLinkStub,
        LocaleSwitcher: { template: '<div data-testid="locale-switcher" />' },
        RainGlyph: { props: ['name'], template: '<i :data-icon="name" />' },
        RainyBackground: { template: '<div data-testid="rain-background" />' },
        GlassPane: { template: '<main data-testid="glass-pane"><slot /></main>' },
      },
    },
  })
}

describe('RainGatewayHome', () => {
  it('keeps configured branding and the existing public routes', () => {
    const wrapper = mountHome()

    expect(wrapper.text()).toContain('Configured Gateway')
    expect(wrapper.text()).toContain('Configured subtitle')
    expect(wrapper.get('img').attributes('src')).toBe('/configured-logo.svg')
    expect(wrapper.get(`a[href="${baseProps.docUrl}"]`).exists()).toBe(true)

    const links = wrapper.findAllComponents(RouterLinkStub)
    expect(links.some((link) => link.props('to') === '/login')).toBe(true)
    const modelPlaza = links.find((link) => link.props('to') === '/model-plaza')
    expect(modelPlaza).toBeDefined()
    expect(modelPlaza?.classes()).not.toContain('hidden')

    expect(wrapper.text()).toContain('POST /v1/messages')
    expect(wrapper.text()).toContain('home.tags.stickySession')
  })

  it('uses the existing dashboard destination for authenticated users', () => {
    const wrapper = mountHome({ isAuthenticated: true, dashboardPath: '/admin', userInitial: 'A' })
    const links = wrapper.findAllComponents(RouterLinkStub)

    expect(links.filter((link) => link.props('to') === '/admin')).toHaveLength(2)
    expect(wrapper.text()).toContain('A')
  })

  it('keeps a configured API suffix while applying the target accent', () => {
    const wrapper = mountHome({ siteName: 'Configured API' })

    expect(wrapper.get('h1').text()).toBe('Configured API')
    expect(wrapper.get('h1 span').text()).toBe('API')
    expect(wrapper.get('h1 span').classes()).toContain('text-cyan-200')
  })

  it('preserves the existing theme event and exposes the target light treatment', async () => {
    const wrapper = mountHome({ isDark: true })
    const themeButton = wrapper.get(`button[aria-label="home.switchToLight"]`)

    await themeButton.trigger('click')
    expect(wrapper.emitted('toggle-theme')).toHaveLength(1)

    await wrapper.setProps({ isDark: false })
    expect(wrapper.get('.rain-gateway-root').classes()).toContain('brightness-[1.1]')
    expect(wrapper.get(`button[aria-label="home.switchToDark"]`).exists()).toBe(true)
  })
})
