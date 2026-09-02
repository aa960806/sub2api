import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { PublicSettings } from '@/types'

const appStoreState = vi.hoisted(() => ({
  publicSettingsLoaded: false,
  cachedPublicSettings: null as Partial<PublicSettings> | null,
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => appStoreState,
}))

import {
  FeatureFlags,
  isFeatureFlagEnabled,
  makeSidebarFlag,
} from '@/utils/featureFlags'

describe('feature flag registry', () => {
  beforeEach(() => {
    appStoreState.publicSettingsLoaded = false
    appStoreState.cachedPublicSettings = null
  })

  it('fails closed while settings are not loaded, including stale cached true values', () => {
    appStoreState.cachedPublicSettings = {
      subnexus_activity_center_enabled: true,
      payment_enabled: true,
    }

    expect(isFeatureFlagEnabled(FeatureFlags.activityCenter)).toBe(false)
    expect(isFeatureFlagEnabled(FeatureFlags.payment)).toBe(false)
    expect(makeSidebarFlag(FeatureFlags.activityCenter)()).toBe(false)
  })

  it('uses an explicit loaded value for opt-in flags', () => {
    appStoreState.publicSettingsLoaded = true
    appStoreState.cachedPublicSettings = {
      subnexus_marquee_enabled: true,
      subnexus_first_recharge_enabled: false,
    }

    expect(isFeatureFlagEnabled(FeatureFlags.marquee)).toBe(true)
    expect(isFeatureFlagEnabled(FeatureFlags.firstRecharge)).toBe(false)
  })

  it('keeps opt-out compatibility only for a successfully loaded payload', () => {
    appStoreState.publicSettingsLoaded = true
    appStoreState.cachedPublicSettings = {}

    expect(isFeatureFlagEnabled(FeatureFlags.channelMonitor)).toBe(true)
    expect(isFeatureFlagEnabled(FeatureFlags.payment)).toBe(true)
  })

  it('registers every staged public migration switch as opt-in', () => {
    const stagedFlags = [
      ['activityCenter', 'subnexus_activity_center_enabled'],
      ['marquee', 'subnexus_marquee_enabled'],
      ['checkIn', 'subnexus_checkin_enabled'],
      ['leaderboard', 'subnexus_leaderboard_enabled'],
      ['inviteActivities', 'subnexus_invite_activities_enabled'],
      ['inviteLottery', 'subnexus_invite_lottery_enabled'],
      ['rechargeWheel', 'subnexus_recharge_wheel_enabled'],
      ['inviteMilestone', 'subnexus_invite_milestone_enabled'],
      ['battlePass', 'battle_pass_enabled'],
      ['invoice', 'invoice_enabled'],
      ['firstRecharge', 'subnexus_first_recharge_enabled'],
      ['studentRechargeBenefit', 'subnexus_student_recharge_benefit_enabled'],
    ] as const

    for (const [name, key] of stagedFlags) {
      expect(FeatureFlags[name]).toMatchObject({ key, mode: 'opt-in' })
    }
  })
})
