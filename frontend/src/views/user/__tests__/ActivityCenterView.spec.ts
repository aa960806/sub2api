import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, shallowMount } from '@vue/test-utils'

const listActivity = vi.hoisted(() => vi.fn())
const showError = vi.hoisted(() => vi.fn())
const routerPush = vi.hoisted(() => vi.fn())
const appStore = vi.hoisted(() => ({
  showError,
  publicSettingsLoaded: true,
  cachedPublicSettings: { subnexus_activity_center_enabled: true },
  fetchPublicSettings: vi.fn(),
}))

vi.mock('@/api/activityCenter', () => ({ listActivityCenter: listActivity }))
vi.mock('@/stores/app', () => ({
  useAppStore: () => appStore,
}))
vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key }),
  }
})
vi.mock('vue-router', () => ({
  useRouter: () => ({ push: routerPush }),
}))

import ActivityCenterView from '../ActivityCenterView.vue'

const baseItem = {
  id: 1,
  slug: 'safe',
  title: 'Safe activity',
  subtitle: '',
  description: 'Description',
  icon: 'gift',
  cover_url: '',
  route_path: '',
  external_url: 'https://example.com/activity',
  action_label: 'Open',
  activity_type: 'custom',
  enabled: true,
  sort_order: 1,
  metadata: {},
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
}

function mountView() {
  return shallowMount(ActivityCenterView, {
    global: {
      stubs: {
        AppLayout: { template: '<div><slot /></div>' },
        Icon: { template: '<span />' },
      },
    },
  })
}

describe('user activity center', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    appStore.publicSettingsLoaded = true
    appStore.cachedPublicSettings = { subnexus_activity_center_enabled: true }
    listActivity.mockResolvedValue({
      enabled: true,
      items: [
        { ...baseItem, id: 2, slug: 'unsafe', title: 'Unsafe activity', external_url: 'javascript:alert(1)' },
        baseItem,
      ],
    })
    vi.stubGlobal('open', vi.fn())
  })

  it('does not render or open unsafe external targets', async () => {
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.text()).toContain('Unsafe activity')
    const buttons = wrapper.findAll('article button')
    expect(buttons).toHaveLength(1)
    await buttons[0].trigger('click')
    expect(window.open).toHaveBeenCalledWith('https://example.com/activity', '_blank', 'noopener,noreferrer')
  })

  it('does not query activity rows while the independent switch is off', async () => {
    appStore.cachedPublicSettings = { subnexus_activity_center_enabled: false }

    const wrapper = mountView()
    await flushPromises()

    expect(listActivity).not.toHaveBeenCalled()
    expect(wrapper.find('[data-testid="activity-center-disabled"]').exists()).toBe(true)
  })
})
