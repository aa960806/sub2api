import { describe, expect, it } from 'vitest'
import { isLeaderboardSettingsEnabled } from '@/utils/leaderboard'

describe('leaderboard feature gate', () => {
  it.each([
    [true, { subnexus_leaderboard_enabled: true }, true],
    [true, { subnexus_leaderboard_enabled: false }, false],
    [true, {}, false],
    [false, { subnexus_leaderboard_enabled: true }, false],
    [false, null, false],
  ])('requires loaded explicit true setting', (loaded, settings, expected) => {
    expect(isLeaderboardSettingsEnabled(loaded, settings)).toBe(expected)
  })
})
