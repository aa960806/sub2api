import type { PublicSettings } from '@/types'

export type InviteActivityPublicFlag =
  | 'subnexus_invite_lottery_enabled'
  | 'subnexus_recharge_wheel_enabled'
  | 'subnexus_invite_milestone_enabled'

type InviteActivitySettings = Pick<
  PublicSettings,
  | 'subnexus_invite_activities_enabled'
  | 'subnexus_invite_lottery_enabled'
  | 'subnexus_recharge_wheel_enabled'
  | 'subnexus_invite_milestone_enabled'
>

/**
 * User activity access is opt-in. An absent payload, a failed load, or either
 * an absent aggregate/child switch must never expose or query the activity.
 */
export function isInviteActivitySettingsEnabled(
  publicSettingsLoaded: boolean,
  settings: InviteActivitySettings | null | undefined,
  childFlag: InviteActivityPublicFlag,
): boolean {
  return (
    publicSettingsLoaded &&
    settings?.subnexus_invite_activities_enabled === true &&
    settings[childFlag] === true
  )
}
