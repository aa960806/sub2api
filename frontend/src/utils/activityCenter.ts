import type { PublicSettings } from '@/types'
import { sanitizeUrl } from './url'

/**
 * Resolve the activity-center opt-in flag without an optimistic fallback.
 * A missing cache, a missing field, and a failed request (loaded=false) are
 * all disabled states.
 */
export function isActivityCenterSettingsEnabled(
  settingsLoaded: boolean,
  settings: Pick<PublicSettings, 'subnexus_activity_center_enabled'> | null | undefined,
): boolean {
  return settingsLoaded && settings?.subnexus_activity_center_enabled === true
}

/** Validate an activity's external target before opening a new window. */
export function isSafeActivityExternalUrl(value: unknown): value is string {
  if (typeof value !== 'string' || !value.trim()) return false
  const sanitized = sanitizeUrl(value)
  if (!sanitized) return false
  try {
    const parsed = new URL(sanitized)
    return (parsed.protocol === 'http:' || parsed.protocol === 'https:') && !parsed.username && !parsed.password
  } catch {
    return false
  }
}

/** Validate an internal activity target as a same-origin absolute path. */
export function isSafeActivityRoutePath(value: unknown): value is string {
  if (typeof value !== 'string' || !value.startsWith('/') || value.startsWith('//')) return false
  if (/[\\\r\n]/.test(value)) return false
  try {
    const parsed = new URL(value, 'https://activity-center.invalid')
    return parsed.origin === 'https://activity-center.invalid'
  } catch {
    return false
  }
}
