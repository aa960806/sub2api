import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import type { MonitorConfig } from '@/api/channelMonitorV2'
import MonitorSettingsPanel from '../MonitorSettingsPanel.vue'

const { getConfig, updateConfig, getGroups, showSuccess, showError } = vi.hoisted(() => ({
  getConfig: vi.fn(),
  updateConfig: vi.fn(),
  getGroups: vi.fn(),
  showSuccess: vi.fn(),
  showError: vi.fn(),
}))

vi.mock('@/api/channelMonitorV2', async (importOriginal) => ({
  ...await importOriginal<typeof import('@/api/channelMonitorV2')>(),
  getConfig,
  updateConfig,
}))
vi.mock('@/api/admin', () => ({ adminAPI: { groups: { getAllIncludingInactive: getGroups } } }))
vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    cachedPublicSettings: { channel_monitor_enabled: true },
    showSuccess,
    showError,
  }),
}))
vi.mock('@/utils/featureFlags', () => ({
  getChannelMonitorMode: () => 'v3',
  isChannelMonitorV2Mode: () => false,
  isChannelMonitorV3Mode: () => true,
}))
vi.mock('vue-i18n', async (importOriginal) => ({
  ...await importOriginal<typeof import('vue-i18n')>(),
  useI18n: () => ({
    t: (key: string, values?: Record<string, unknown>) => values ? `${key} ${JSON.stringify(values)}` : key,
    te: () => false,
  }),
}))

const groupRows = [
  { id: 13, platform: 'openai', name: 'OpenAI 13' },
  { id: 21, platform: 'anthropic', name: 'Claude 21' },
  { id: 11, platform: 'openai', name: 'OpenAI 11' },
  { id: 12, platform: 'openai', name: 'OpenAI 12' },
]

function initialConfig(): MonitorConfig {
  return {
    version: 7,
    enabled: true,
    refresh_interval_seconds: 60,
    platforms: [
      { platform: 'openai', enabled: true, models: ['gpt-5'], group_order: [12, 11] },
      { platform: 'anthropic', enabled: true, models: [] },
      { platform: 'kimi', enabled: false, models: [] },
    ],
    group_ids: [],
    health_thresholds: {
      minimum_sample: 50,
      warning_error_rate: 0.05,
      critical_error_rate: 0.2,
      target_ttft_ms: 3000,
      warning_ttft_ms: 3000,
      critical_ttft_ms: 10000,
      warning_cache_rate: 0.85,
      critical_cache_rate: 0.6,
      error_weight: 0.6,
      ttft_weight: 0.2,
      cache_weight: 0.2,
    },
    ignored_error_categories: [],
  }
}

function mountPanel() {
  return mount(MonitorSettingsPanel, { global: { stubs: { Icon: true, RouterLink: true } } })
}

function vendors(wrapper: VueWrapper) {
  return wrapper.findAll('[data-vendor]').map((row) => row.attributes('data-vendor'))
}

function orderedGroups(wrapper: VueWrapper, vendor = 'openai') {
  return wrapper.get(`[data-vendor="${vendor}"]`).findAll('[data-ordered-group]')
    .map((row) => Number(row.attributes('data-ordered-group')))
}

describe('MonitorSettingsPanel display order', () => {
  let savedConfig: MonitorConfig

  beforeEach(() => {
    vi.clearAllMocks()
    savedConfig = initialConfig()
    getConfig.mockImplementation(async () => structuredClone(savedConfig))
    getGroups.mockResolvedValue(groupRows)
    updateConfig.mockImplementation(async (payload: MonitorConfig) => {
      savedConfig = JSON.parse(JSON.stringify({ ...payload, version: payload.version + 1 }))
      return structuredClone(savedConfig)
    })
  })

  it('saves vendor and group order with the current version, preserves all-group scope, and reloads it', async () => {
    const wrapper = mountPanel()
    await flushPromises()
    expect(orderedGroups(wrapper)).toEqual([12, 11, 13])
    expect(wrapper.get('[data-testid="monitor-settings-save"]').attributes('disabled')).toBeDefined()
    expect(wrapper.get('[data-vendor="openai"] [data-action="vendor-up"]').attributes('disabled')).toBeDefined()
    expect(wrapper.get('[data-vendor="kimi"] [data-action="vendor-down"]').attributes('disabled')).toBeDefined()

    await wrapper.get('[data-vendor="anthropic"] [data-action="vendor-up"]').trigger('click')
    wrapper.get('[data-vendor="openai"] details').element.setAttribute('open', '')
    await wrapper.get('[data-ordered-group="13"] [data-action="group-up"]').trigger('click')
    expect(vendors(wrapper)).toEqual(['anthropic', 'openai', 'kimi'])
    expect(orderedGroups(wrapper)).toEqual([12, 13, 11])
    expect(wrapper.findAll<HTMLInputElement>('[data-group-selection]').every((input) => !input.element.checked)).toBe(true)

    await wrapper.get('[data-testid="monitor-settings-save"]').trigger('click')
    await flushPromises()
    expect(updateConfig).toHaveBeenCalledWith(expect.objectContaining({
      version: 7,
      group_ids: [],
      platforms: [
        { platform: 'anthropic', enabled: true, models: [] },
        { platform: 'openai', enabled: true, models: ['gpt-5'], group_order: [12, 13, 11] },
        { platform: 'kimi', enabled: false, models: [] },
      ],
    }))
    expect(showSuccess).toHaveBeenCalledOnce()
    expect(wrapper.get('[data-testid="monitor-settings-save"]').attributes('disabled')).toBeDefined()
    wrapper.unmount()

    const reloaded = mountPanel()
    await flushPromises()
    expect(vendors(reloaded)).toEqual(['anthropic', 'openai', 'kimi'])
    expect(orderedGroups(reloaded)).toEqual([12, 13, 11])
    await reloaded.get('[data-vendor="openai"] [data-action="vendor-up"]').trigger('click')
    await reloaded.get('[data-testid="monitor-settings-save"]').trigger('click')
    await flushPromises()
    expect(updateConfig).toHaveBeenLastCalledWith(expect.objectContaining({ version: 8, group_ids: [] }))
    reloaded.unmount()
  })

  it('reorders selected groups without changing selection or dropping excluded group positions', async () => {
    savedConfig.group_ids = [11, 13, 21]
    savedConfig.platforms[0].group_order = [11, 12, 13]
    const wrapper = mountPanel()
    await flushPromises()
    expect(orderedGroups(wrapper)).toEqual([11, 13])
    await wrapper.get('[data-ordered-group="13"] [data-action="group-up"]').trigger('click')
    expect(orderedGroups(wrapper)).toEqual([13, 11])
    expect(wrapper.get<HTMLInputElement>('[data-group-selection="12"]').element.checked).toBe(false)

    await wrapper.get('[data-testid="monitor-settings-save"]').trigger('click')
    await flushPromises()
    expect(savedConfig.group_ids).toEqual([11, 13, 21])
    expect(savedConfig.platforms[0].group_order).toEqual([13, 12, 11])

    await wrapper.get('[data-group-selection="12"]').setValue(true)
    expect(orderedGroups(wrapper)).toEqual([13, 12, 11])
    expect(wrapper.get<HTMLInputElement>('[data-group-selection="12"]').element.checked).toBe(true)
    wrapper.unmount()
  })

  it('lets legacy configurations initialize group order without changing their group scope', async () => {
    delete savedConfig.platforms[0].group_order
    const wrapper = mountPanel()
    await flushPromises()
    expect(orderedGroups(wrapper)).toEqual([11, 12, 13])
    await wrapper.get('[data-ordered-group="11"] [data-action="group-down"]').trigger('click')
    await wrapper.get('[data-testid="monitor-settings-save"]').trigger('click')
    await flushPromises()
    expect(savedConfig.platforms[0].group_order).toEqual([12, 11, 13])
    expect(savedConfig.group_ids).toEqual([])
    wrapper.unmount()
  })

  it('orders selected composite groups independently under each destination vendor', async () => {
    getGroups.mockResolvedValue([...groupRows, { id: 30, platform: 'composite', name: 'Multi vendor' }])
    savedConfig.group_ids = [11, 21, 30]
    const wrapper = mountPanel()
    await flushPromises()
    expect(orderedGroups(wrapper)).toEqual([11, 30])
    expect(orderedGroups(wrapper, 'anthropic')).toEqual([21, 30])
    expect(wrapper.get('[data-vendor="openai"]').text()).toContain('channelMonitorV3.settings.compositeOrderHint')
    await wrapper.get('[data-vendor="openai"] [data-ordered-group="30"] [data-action="group-up"]').trigger('click')
    expect(orderedGroups(wrapper)).toEqual([30, 11])
    expect(orderedGroups(wrapper, 'anthropic')).toEqual([21, 30])
    await wrapper.get('[data-testid="monitor-settings-save"]').trigger('click')
    await flushPromises()
    expect(savedConfig.group_ids).toEqual([11, 21, 30])
    expect(savedConfig.platforms[0].group_order).toEqual([12, 30, 11])
    expect(savedConfig.platforms[1].group_order).toBeUndefined()

    await wrapper.get('[data-group-selection="30"]').setValue(false)
    expect(orderedGroups(wrapper)).toEqual([11])
    expect(orderedGroups(wrapper, 'anthropic')).toEqual([21])
    expect(wrapper.get('[data-vendor="openai"]').text()).not.toContain('channelMonitorV3.settings.compositeOrderHint')
    wrapper.unmount()
  })

  it('uses the provider catalog label for OpenCode', async () => {
    savedConfig.platforms.push({ platform: 'opencode_go', enabled: true, models: [] })
    const wrapper = mountPanel()
    await flushPromises()
    expect(wrapper.get('[data-vendor="opencode_go"] strong').text()).toBe('OpenCode')
    wrapper.unmount()
  })

  it('shows disabled and empty vendor hints while allowing preparation of its vendor position', async () => {
    const wrapper = mountPanel()
    await flushPromises()
    const kimi = wrapper.get('[data-vendor="kimi"]')
    expect(kimi.text()).toContain('channelMonitorV3.settings.vendorDisabled')
    expect(kimi.text()).toContain('channelMonitorV3.settings.groupOrderEmpty')
    await kimi.get('[data-action="vendor-up"]').trigger('click')
    expect(vendors(wrapper)).toEqual(['openai', 'kimi', 'anthropic'])
    await wrapper.get('[data-testid="monitor-settings-save"]').trigger('click')
    await flushPromises()
    expect(savedConfig.platforms[1]).toEqual({ platform: 'kimi', enabled: false, models: [] })
    wrapper.unmount()
  })

  it('reloads authoritative ordering after a version conflict', async () => {
    const wrapper = mountPanel()
    await flushPromises()
    await wrapper.get('[data-vendor="anthropic"] [data-action="vendor-up"]').trigger('click')
    savedConfig.version = 9
    savedConfig.platforms[0].group_order = [13, 12, 11]
    updateConfig.mockRejectedValueOnce(new Error('version conflict'))
    await wrapper.get('[data-testid="monitor-settings-save"]').trigger('click')
    await flushPromises()
    expect(showError).toHaveBeenCalledOnce()
    expect(getConfig).toHaveBeenCalledTimes(2)
    expect(vendors(wrapper)).toEqual(['openai', 'anthropic', 'kimi'])
    expect(orderedGroups(wrapper)).toEqual([13, 12, 11])
    expect(wrapper.get('[data-testid="monitor-settings-save"]').attributes('disabled')).toBeDefined()
    wrapper.unmount()
  })
})
