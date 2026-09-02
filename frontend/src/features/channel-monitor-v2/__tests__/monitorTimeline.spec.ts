import { describe, expect, it } from 'vitest'
import { buildMonitorTimeline } from '../monitorTimeline'

interface TestBucket {
  bucket_start: string
  value: number
}

describe('buildMonitorTimeline', () => {
  it('keeps the trailing slot visible when the current bucket has no data yet', () => {
    const buckets: TestBucket[] = [
      { bucket_start: '2026-08-31T19:25:00Z', value: 1 },
      { bucket_start: '2026-08-31T19:30:00Z', value: 2 },
    ]
    const slots = buildMonitorTimeline(buckets, 3, '2026-08-31T19:40:00Z', 300)

    expect(slots.map(slot => new Date(slot.timestamp).toISOString())).toEqual([
      '2026-08-31T19:25:00.000Z',
      '2026-08-31T19:30:00.000Z',
      '2026-08-31T19:35:00.000Z',
    ])
    expect(slots[0].bucket?.value).toBe(1)
    expect(slots[1].bucket?.value).toBe(2)
    expect(slots[2].bucket).toBeUndefined()
  })

  it('always returns the requested number of fixed slots for empty data', () => {
    const slots = buildMonitorTimeline<TestBucket>([], 18, '2026-08-31T20:00:00Z', 300)

    expect(slots).toHaveLength(18)
    expect(slots.every(slot => slot.bucket === undefined)).toBe(true)
    expect(slots[17].timestamp).toBe(Date.parse('2026-08-31T19:55:00Z'))
  })

  it('filters invalid timestamps and maps valid buckets to their interval', () => {
    const buckets: TestBucket[] = [
      { bucket_start: 'not-a-date', value: 0 },
      { bucket_start: '2026-08-31T19:30:00Z', value: 2 },
    ]
    const slots = buildMonitorTimeline(buckets, 2, '2026-08-31T19:40:00Z', 300)

    expect(slots[0].bucket?.value).toBe(2)
    expect(slots[1].bucket).toBeUndefined()
  })
})
