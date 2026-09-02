import type { PublicSettings } from '@/types'

/**
 * Resolve the leaderboard route gate. A successful settings load and an
 * explicit boolean true are both required; stale or missing payloads remain
 * disabled during the staged migration.
 */
export function isLeaderboardSettingsEnabled(
  settingsLoaded: boolean,
  settings: Pick<PublicSettings, 'subnexus_leaderboard_enabled'> | null | undefined,
): boolean {
  return settingsLoaded && settings?.subnexus_leaderboard_enabled === true
}
