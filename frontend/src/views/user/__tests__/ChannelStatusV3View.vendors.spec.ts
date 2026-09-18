import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { MonitorConfig, MonitorMatrixResponse, MonitorMatrixRow, MonitorSnapshot } from '@/api/channelMonitorV2'
import ChannelStatusV3View from '../ChannelStatusV3View.vue'

const { getSnapshot, getMatrix, getAvailable, getUserGroupRates, showError } = vi.hoisted(() => ({
  getSnapshot: vi.fn(),
  getMatrix: vi.fn(),
  getAvailable: vi.fn(),
  getUserGroupRates: vi.fn(),
  showError: vi.fn(),
}))

vi.mock('@/api/channelMonitorV2', () => ({ getSnapshot, getMatrix }))
vi.mock('@/api/groups', () => ({ default: { getAvailable, getUserGroupRates } }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showError }) }))
vi.mock('vue-i18n', async () => ({
  ...await vi.importActual<typeof import('vue-i18n')>('vue-i18n'),
  useI18n: () => ({ t: (key: string) => key, locale: { value: 'en' } }),
}))

const coverage: MonitorSnapshot['coverage'] = {
  requested_start: '2026-09-18T00:00:00Z',
  requested_end: '2026-09-18T01:30:00Z',
  coverage_start: '2026-09-18T00:00:00Z',
  data_through: '2026-09-18T01:30:00Z',
  computed_at: '2026-09-18T01:30:00Z',
  aggregation_lag_seconds: 0,
  coverage_complete: true,
  bucket_seconds: 300,
}
const metrics: MonitorSnapshot['metrics'] = {
  success_requests: 10,
  error_requests: 0,
  request_count: 10,
  token_count: 100,
  rpm: 1,
  tpm: 10,
  error_rate: 0,
  cache_rate: 0.5,
  cache_rate_numerator: 50,
  cache_rate_denominator: 100,
  ttft: { sample_count: 10, p50_ms: 100, p95_ms: 200, avg_ms: 150 },
  duration: { sample_count: 10, p50_ms: 500, p95_ms: 900, avg_ms: 600 },
}
const health: MonitorSnapshot['health'] = {
  overall: 'healthy', error_rate: 'healthy', ttft: 'healthy', minimum_sample: 1,
}

function platform(platform: string, group_order?: number[]): MonitorConfig['platforms'][number] {
  return { platform, enabled: true, models: [], group_order }
}

function snapshot(platforms: MonitorConfig['platforms']): MonitorSnapshot {
  return {
    config: {
      version: 2,
      enabled: true,
      refresh_interval_seconds: 60,
      platforms,
      group_ids: [11, 12, 20, 21, 30, 40],
      health_thresholds: {
        minimum_sample: 1,
        warning_error_rate: 0.1,
        critical_error_rate: 0.5,
        target_ttft_ms: 100,
        warning_ttft_ms: 1000,
        critical_ttft_ms: 5000,
        warning_cache_rate: 0.2,
        critical_cache_rate: 0.1,
        error_weight: 0.5,
        ttft_weight: 0.3,
        cache_weight: 0.2,
      },
    },
    coverage,
    metrics,
    health,
    trend: [],
  }
}

function row(platform: string, group_id: number): MonitorMatrixRow {
  return { platform, group_id, group_name: `${platform} group ${group_id}`, metrics, health, buckets: [] }
}

const mountView = () => mount(ChannelStatusV3View, {
  global: {
    stubs: {
      AppLayout: { template: '<main><slot /></main>' },
      Icon: true,
      EmptyState: true,
      ChannelMonitorV3Card: {
        props: ['row', 'timelineLength', 'timelineEndAt', 'timelineBucketSeconds'],
        template: '<article :data-group-id="row.group_id" :data-timeline-length="timelineLength">{{ row.group_name }}</article>',
      },
    },
  },
})

let wrapper: ReturnType<typeof mountView> | undefined

function vendorOrder() {
  return wrapper!.findAll('section[data-vendor]').map(section => section.attributes('data-vendor'))
}

function groupOrder(vendor: string) {
  return wrapper!.get(`section[data-vendor="${vendor}"]`).findAll('[data-group-id]')
    .map(card => Number(card.attributes('data-group-id')))
}

beforeEach(() => {
  vi.useFakeTimers()
  vi.clearAllMocks()
  vi.spyOn(document, 'hidden', 'get').mockReturnValue(false)
  getSnapshot.mockReset().mockResolvedValue(snapshot([
    platform('zhipu', [30]),
    platform('kimi', [999, 12, 11]),
    platform('minimax', [40]),
    platform('deepseek', [21, 20]),
    platform('openai', [888]),
  ]))
  getMatrix.mockReset().mockResolvedValue({
    coverage,
    group_by: 'platform_group',
    items: [row('deepseek', 20), row('kimi', 11), row('zhipu', 30), row('minimax', 40), row('kimi', 12), row('deepseek', 21)],
  } satisfies MonitorMatrixResponse)
  // Pricing access must not add cards absent from the monitor matrix.
  getAvailable.mockResolvedValue([{ id: 999, rate_multiplier: 1 }])
  getUserGroupRates.mockResolvedValue({})
})

afterEach(() => {
  wrapper?.unmount()
  wrapper = undefined
  vi.restoreAllMocks()
  vi.useRealTimers()
})

describe('ChannelStatusV3View vendor sections', () => {
  it('renders domestic vendors and their matrix groups in the administrator-defined order', async () => {
    wrapper = mountView()
    await flushPromises()

    expect(vendorOrder()).toEqual(['zhipu', 'kimi', 'minimax', 'deepseek'])
    expect(groupOrder('zhipu')).toEqual([30])
    expect(groupOrder('kimi')).toEqual([12, 11])
    expect(groupOrder('minimax')).toEqual([40])
    expect(groupOrder('deepseek')).toEqual([21, 20])
    for (const vendor of vendorOrder()) {
      const section = wrapper.get(`section[data-vendor="${vendor}"]`)
      expect(section.get('h2').text()).toContain(`monitorCommon.providers.${vendor}`)
      expect(section.attributes('aria-label')).toBe(`monitorCommon.providers.${vendor}`)
    }
    expect(wrapper.findAll('[data-group-id]')).toHaveLength(6)
    expect(wrapper.find('[data-group-id="999"]').exists()).toBe(false)
    expect(wrapper.find('[data-group-id="888"]').exists()).toBe(false)
    expect(wrapper.find('[data-vendor="openai"]').exists()).toBe(false)
    expect(showError).not.toHaveBeenCalled()
  })

  it('applies refreshed vendor and group order without adding configured-only groups', async () => {
    wrapper = mountView()
    await flushPromises()
    getSnapshot.mockResolvedValue(snapshot([
      platform('deepseek', [20, 21]),
      platform('minimax', [40]),
      platform('kimi', [11, 999, 12]),
      platform('zhipu', [30]),
    ]))

    await wrapper.get('button[title="common.refresh"]').trigger('click')
    await flushPromises()

    expect(getSnapshot).toHaveBeenCalledTimes(2)
    expect(getMatrix).toHaveBeenCalledTimes(2)
    expect(vendorOrder()).toEqual(['deepseek', 'minimax', 'kimi', 'zhipu'])
    expect(groupOrder('deepseek')).toEqual([20, 21])
    expect(groupOrder('kimi')).toEqual([11, 12])
    expect(wrapper.findAll('[data-group-id]')).toHaveLength(6)
  })

  it('retains the selected range and platform-group scope during automatic refresh', async () => {
    wrapper = mountView()
    await flushPromises()
    const rangeButton = wrapper.findAll('button').find(button => button.text() === 'channelMonitorV3.ranges.7d')!
    await rangeButton.trigger('click')
    await flushPromises()

    const filter = { range: '7d', platforms: [], groupIds: [], models: [] }
    expect(getSnapshot).toHaveBeenLastCalledWith(filter, false, expect.any(AbortSignal))
    expect(getMatrix).toHaveBeenLastCalledWith(filter, 'platform_group', false, expect.any(AbortSignal))
    expect(wrapper.findAll('[data-timeline-length="14"]')).toHaveLength(6)

    await vi.advanceTimersByTimeAsync(60000)
    await flushPromises()

    expect(getSnapshot).toHaveBeenCalledTimes(3)
    expect(getMatrix).toHaveBeenCalledTimes(3)
    expect(getMatrix).toHaveBeenLastCalledWith(filter, 'platform_group', false, expect.any(AbortSignal))
    expect(vendorOrder()).toEqual(['zhipu', 'kimi', 'minimax', 'deepseek'])
    expect(groupOrder('kimi')).toEqual([12, 11])
  })
})
