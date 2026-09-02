import { describe, expect, it } from 'vitest'
import {
  isActivityCenterSettingsEnabled,
  isSafeActivityExternalUrl,
  isSafeActivityRoutePath,
} from '@/utils/activityCenter'

describe('activity center safety helpers', () => {
  it.each([
    [true, { subnexus_activity_center_enabled: true }, true],
    [true, { subnexus_activity_center_enabled: false }, false],
    [true, {}, false],
    [false, { subnexus_activity_center_enabled: true }, false],
    [false, null, false],
  ])('allows the route only for a loaded explicit true flag', (loaded, settings, expected) => {
    expect(isActivityCenterSettingsEnabled(loaded, settings)).toBe(expected)
  })

  it('accepts only http(s) external URLs without credentials', () => {
    expect(isSafeActivityExternalUrl('https://example.com/event')).toBe(true)
    expect(isSafeActivityExternalUrl('http://example.com/event?q=1')).toBe(true)
    expect(isSafeActivityExternalUrl('javascript:alert(1)')).toBe(false)
    expect(isSafeActivityExternalUrl('//example.com/event')).toBe(false)
    expect(isSafeActivityExternalUrl('https://user:pass@example.com/event')).toBe(false)
  })

  it('accepts same-origin style local paths only', () => {
    expect(isSafeActivityRoutePath('/activities/demo')).toBe(true)
    expect(isSafeActivityRoutePath('/activities/demo?tab=1#details')).toBe(true)
    expect(isSafeActivityRoutePath('https://example.com')).toBe(false)
    expect(isSafeActivityRoutePath('//example.com')).toBe(false)
    expect(isSafeActivityRoutePath('/activities\\demo')).toBe(false)
  })
})
