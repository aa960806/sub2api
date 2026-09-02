import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, shallowMount } from '@vue/test-utils'

const getConfig = vi.hoisted(() => vi.fn())
const updateConfig = vi.hoisted(() => vi.fn())
const listAdmin = vi.hoisted(() => vi.fn())
const createItem = vi.hoisted(() => vi.fn())
const updateItem = vi.hoisted(() => vi.fn())
const deleteItem = vi.hoisted(() => vi.fn())
const showError = vi.hoisted(() => vi.fn())
const showSuccess = vi.hoisted(() => vi.fn())

vi.mock('@/api/activityCenter', () => ({
  getActivityCenterConfig: getConfig,
  updateActivityCenterConfig: updateConfig,
  listAdminActivityCenterItems: listAdmin,
  createActivityCenterItem: createItem,
  updateActivityCenterItem: updateItem,
  deleteActivityCenterItem: deleteItem,
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    cachedPublicSettings: null,
    showError,
    showSuccess,
  }),
}))

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key }),
  }
})

import ActivityCenterView from '../ActivityCenterView.vue'

const customItem = {
  id: 1,
  slug: 'custom-one',
  title: 'Custom one',
  subtitle: '',
  description: '',
  icon: 'gift',
  cover_url: '',
  route_path: '/custom/one',
  external_url: '',
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
        Toggle: {
          props: ['modelValue', 'disabled'],
          emits: ['update:modelValue'],
          template: '<button type="button" :disabled="disabled" />',
        },
        ConfirmDialog: { template: '<div />' },
      },
    },
  })
}

describe('admin activity center closed state', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    getConfig.mockResolvedValue({ enabled: false })
    listAdmin.mockResolvedValue([customItem])
  })

  it('does not request items while the switch is off and disables CRUD', async () => {
    const wrapper = mountView()
    await flushPromises()

    expect(getConfig).toHaveBeenCalledOnce()
    expect(listAdmin).not.toHaveBeenCalled()
    expect(wrapper.get('[data-testid="activity-center-closed-notice"]').exists()).toBe(true)
    expect(wrapper.get('[data-testid="activity-center-save"]').attributes('disabled')).toBeDefined()
  })

  it('renders custom entries only after the switch is enabled', async () => {
    getConfig.mockResolvedValue({ enabled: true })
    listAdmin.mockResolvedValue([
      customItem,
      { ...customItem, id: 2, slug: 'legacy', title: 'Legacy non-custom', activity_type: 'battle_pass' },
    ])

    const wrapper = mountView()
    await flushPromises()

    expect(listAdmin).toHaveBeenCalledOnce()
    expect(wrapper.text()).toContain('Custom one')
    expect(wrapper.text()).not.toContain('Legacy non-custom')
  })
})
