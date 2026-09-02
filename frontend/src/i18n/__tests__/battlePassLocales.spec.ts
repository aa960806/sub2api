import { describe, expect, it } from 'vitest'
import en from '../locales/en/battlePass'
import zh from '../locales/zh/battlePass'

describe('battle pass locale modules', () => {
  it('keeps the English and Chinese message contracts aligned', () => {
    expect(Object.keys(en.battlePass).sort()).toEqual(Object.keys(zh.battlePass).sort())
  })

  it('describes the independent user-side gate without requiring a legacy activity card', () => {
    expect(en.battlePass.adminEnabledHint).not.toContain('activity-center card')
    expect(zh.battlePass.adminEnabledHint).not.toContain('活动中心')
  })
})
