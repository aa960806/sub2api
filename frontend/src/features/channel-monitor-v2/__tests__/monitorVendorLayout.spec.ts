import { describe, expect, it } from 'vitest'
import type { MonitorMatrixRow } from '@/api/channelMonitorV2'
import { buildMonitorVendorSections } from '../monitorVendorLayout'

const row = (platform: string, group_id?: number) => ({ platform, group_id } as MonitorMatrixRow)

describe('monitor vendor layout', () => {
  it('keeps domestic vendors and respects vendor and per-vendor group ordering', () => {
    const rows = [row('openai', 1), row('kimi', 4), row('openai', 2), row('deepseek', 5), row('zhipu', 6), row('minimax', 7)]
    const sections = buildMonitorVendorSections(rows, [
      { platform: 'deepseek', enabled: true, models: [] },
      { platform: 'openai', enabled: true, models: [], group_order: [2, 1] },
      { platform: 'kimi', enabled: true, models: [] },
    ])
    expect(sections.map(section => section.platform)).toEqual(['deepseek', 'openai', 'kimi', 'minimax', 'zhipu'])
    expect(sections[1].rows.map(item => item.group_id)).toEqual([2, 1])
    expect(rows.map(item => item.group_id)).toEqual([1, 4, 2, 5, 6, 7])
  })

  it('never creates inaccessible rows and appends newly visible groups deterministically', () => {
    const sections = buildMonitorVendorSections([row('openai', 8), row('openai', 3), row('openai', 1)], [
      { platform: 'openai', enabled: true, models: [], group_order: [999, 3] },
      { platform: 'kimi', enabled: true, models: [], group_order: [4] },
    ])
    expect(sections).toHaveLength(1)
    expect(sections[0].rows.map(item => item.group_id)).toEqual([3, 1, 8])
  })

  it('handles old configurations and filters platform placeholders without dropping unknown vendors', () => {
    const sections = buildMonitorVendorSections([row('future', 9), row('kimi', 2), row('kimi', 1), row('openai'), row('kimi', 0)])
    expect(sections.map(section => section.platform)).toEqual(['future', 'kimi'])
    expect(sections[1].rows.map(item => item.group_id)).toEqual([1, 2])
  })
})
