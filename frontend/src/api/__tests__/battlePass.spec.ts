import { describe, expect, it } from 'vitest'
import { battlePassLevelProgress } from '../battlePass'

describe('battlePassLevelProgress', () => {
  it('uses published level thresholds for an in-level progress bar', () => {
    expect(battlePassLevelProgress({
      exp: 175,
      level: 2,
      level_start_exp: 100,
      next_level_exp: 250,
      premium_unlocked: false,
      updated_at: '',
    })).toBe(50)
  })

  it('caps completed levels and handles missing progress safely', () => {
    expect(battlePassLevelProgress({
      exp: 400,
      level: 3,
      level_start_exp: 250,
      next_level_exp: null,
      premium_unlocked: false,
      updated_at: '',
    })).toBe(100)
    expect(battlePassLevelProgress(null)).toBe(0)
  })

  it('clamps malformed values instead of overflowing the bar', () => {
    expect(battlePassLevelProgress({
      exp: 50,
      level: 2,
      level_start_exp: 100,
      next_level_exp: 250,
      premium_unlocked: false,
      updated_at: '',
    })).toBe(0)
  })
})
