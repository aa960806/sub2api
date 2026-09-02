import { describe, expect, it } from 'vitest'
import { isInviteActivitySettingsEnabled } from '@/utils/inviteActivities'

describe('invite activity public settings gate', () => {
  it.each([
    ['settings have not loaded', false, null],
    ['settings are missing', true, null],
    ['aggregate flag is missing', true, { subnexus_invite_lottery_enabled: true }],
    ['aggregate flag is off', true, { subnexus_invite_activities_enabled: false, subnexus_invite_lottery_enabled: true }],
    ['child flag is missing', true, { subnexus_invite_activities_enabled: true }],
    ['child flag is off', true, { subnexus_invite_activities_enabled: true, subnexus_invite_lottery_enabled: false }],
  ])('fails closed when %s', (_case, loaded, settings) => {
    expect(isInviteActivitySettingsEnabled(loaded, settings, 'subnexus_invite_lottery_enabled')).toBe(false)
  })

  it('requires both the aggregate and selected child flag', () => {
    const settings = {
      subnexus_invite_activities_enabled: true,
      subnexus_invite_lottery_enabled: true,
      subnexus_recharge_wheel_enabled: false,
      subnexus_invite_milestone_enabled: true,
    }

    expect(isInviteActivitySettingsEnabled(true, settings, 'subnexus_invite_lottery_enabled')).toBe(true)
    expect(isInviteActivitySettingsEnabled(true, settings, 'subnexus_recharge_wheel_enabled')).toBe(false)
    expect(isInviteActivitySettingsEnabled(true, settings, 'subnexus_invite_milestone_enabled')).toBe(true)
  })
})
