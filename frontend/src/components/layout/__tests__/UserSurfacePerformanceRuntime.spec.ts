import { mount, RouterLinkStub, type VueWrapper } from '@vue/test-utils'
import { nextTick, reactive } from 'vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import AppLayout from '../AppLayout.vue'
import RainyBackground from '@/components/home/RainyBackground.vue'
import { useUserSurfacePerformance } from '@/composables/useUserSurfacePerformance'

const route = reactive({ meta: { requiresAuth: true, requiresAdmin: false } })
const user = { role: 'user', email: 'surface@example.test', balance: 10 }
vi.mock('vue-router', async importOriginal => ({ ...await importOriginal<typeof import('vue-router')>(), useRoute: () => route, useRouter: () => ({ push: vi.fn() }) }))
vi.mock('vue-i18n', async importOriginal => ({ ...await importOriginal<typeof import('vue-i18n')>(), useI18n: () => ({ t: (key: string) => key }) }))
vi.mock('@/stores', () => ({
  useAppStore: () => ({ sidebarCollapsed: false, docUrl: '', contactInfo: '' }),
  useAuthStore: () => ({ user }),
  useOnboardingStore: () => ({ setReplayCallback: vi.fn() }),
}))
vi.mock('@/stores/auth', () => ({ useAuthStore: () => ({ user }) }))
vi.mock('@/stores/adminSettings', () => ({ useAdminSettingsStore: () => ({}) }))
vi.mock('@/stores/onboarding', () => ({ useOnboardingStore: () => ({ setReplayCallback: vi.fn() }) }))
vi.mock('@/composables/useOnboardingTour', () => ({ useOnboardingTour: () => ({ replayTour: vi.fn() }) }))
vi.mock('@/utils/featureFlags', () => ({ FeatureFlags: {}, isFeatureFlagEnabled: () => false }))

let wrapper: VueWrapper | undefined
let frames: Map<number, FrameRequestCallback>
let nextFrame: number
beforeEach(() => {
  localStorage.clear()
  route.meta.requiresAuth = true
  route.meta.requiresAdmin = false
  frames = new Map()
  nextFrame = 0
  vi.spyOn(window, 'matchMedia').mockImplementation(query => ({
    matches: false, media: query, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  } as unknown as MediaQueryList))
  vi.spyOn(document, 'visibilityState', 'get').mockReturnValue('visible')
  vi.spyOn(navigator, 'hardwareConcurrency', 'get').mockReturnValue(8)
  vi.stubGlobal('requestAnimationFrame', vi.fn((callback: FrameRequestCallback) => {
    frames.set(++nextFrame, callback)
    return nextFrame
  }))
  vi.stubGlobal('cancelAnimationFrame', vi.fn((id: number) => frames.delete(id)))
  useUserSurfacePerformance().setMode('auto')
})
afterEach(() => {
  wrapper?.unmount()
  wrapper = undefined
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})

function mountLayout() {
  wrapper = mount(AppLayout, { slots: { default: '<button id="business-action">Existing action</button>' }, global: { stubs: {
    AppSidebar: true, RainyBackground: true, Icon: true, AnnouncementBell: true,
    LocaleSwitcher: true, SubscriptionProgressMini: true, RouterLink: RouterLinkStub,
    Transition: { template: '<div><slot /></div>' },
  } } })
  return wrapper
}

describe('user visual mode integration', () => {
  it('selects modes through the real header and keeps the same content mounted', async () => {
    const layout = mountLayout()
    await nextTick()
    const action = layout.get('#business-action').element
    await layout.get('button[aria-label="common.userMenu"]').trigger('click')
    const select = layout.get('select[aria-label="common.visualMode"]')
    expect(select.findAll('option').map(option => option.attributes('value'))).toEqual(['auto', 'standard', 'balanced', 'compat'])
    await select.setValue('balanced')
    expect(layout.classes()).toContain('user-glass-performance-balanced')
    expect(layout.findComponent(RainyBackground).props()).toMatchObject({ animated: false, quality: 'balanced' })
    expect(frames.size).toBe(0)
    await select.setValue('compat')
    expect(layout.classes()).not.toContain('user-glass-surface')
    expect(layout.findComponent(RainyBackground).exists()).toBe(false)
    expect(layout.find('.user-surface-wash').exists()).toBe(false)
    expect(layout.find('.bg-mesh-gradient').exists()).toBe(true)
    expect(layout.get('#business-action').element).toBe(action)
    await select.setValue('standard')
    expect(layout.findComponent(RainyBackground).props('animated')).toBe(true)
    expect(frames.size).toBe(0) // Manual mode overrides the automatic monitor.
    await select.setValue('auto')
    expect(localStorage.getItem('subnexus_user_surface_mode')).toBeNull()
    expect(frames.size).toBe(1)
  })

  it('detaches pointer work and the monitor when the route becomes administrative', async () => {
    const layout = mountLayout()
    await nextTick()
    window.dispatchEvent(new MouseEvent('pointermove', { clientX: 14, clientY: 25 }))
    expect(frames.size).toBe(2)
    route.meta.requiresAdmin = true
    await nextTick()
    expect(layout.findComponent(RainyBackground).exists()).toBe(false)
    expect(layout.classes().some(value => value.startsWith('user-glass'))).toBe(false)
    expect(frames.size).toBe(0)
    window.dispatchEvent(new MouseEvent('pointermove', { clientX: 18, clientY: 30 }))
    expect(frames.size).toBe(0)
    await layout.get('button[aria-label="common.userMenu"]').trigger('click')
    expect(layout.find('select[aria-label="common.visualMode"]').exists()).toBe(false)
    route.meta.requiresAdmin = false
    route.meta.requiresAuth = false
    await nextTick()
    expect(layout.findComponent(RainyBackground).exists()).toBe(false)
  })

  it('pauses on hidden pages and cleans up every frame on unmount', async () => {
    const layout = mountLayout()
    await nextTick()
    window.dispatchEvent(new MouseEvent('pointermove'))
    vi.spyOn(document, 'visibilityState', 'get').mockReturnValue('hidden')
    document.dispatchEvent(new Event('visibilitychange'))
    await nextTick()
    expect(frames.size).toBe(0)
    expect(layout.findComponent(RainyBackground).props('animated')).toBe(false)
    vi.spyOn(document, 'visibilityState', 'get').mockReturnValue('visible')
    document.dispatchEvent(new Event('visibilitychange'))
    await nextTick()
    expect(frames.size).toBe(1)
    layout.unmount()
    wrapper = undefined
    expect(frames.size).toBe(0)
  })
})
