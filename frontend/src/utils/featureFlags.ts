/**
 * Feature flag registry — single source of truth for public-settings-driven
 * feature switches used by the sidebar, routes, and views.
 *
 * ## Why this module exists
 *
 * `public settings` reach the frontend through two channels:
 *
 *   1. **SSR injection** — the backend embeds `window.__APP_CONFIG__` into the
 *      HTML. `main.ts` calls `appStore.initFromInjectedConfig()` synchronously
 *      before Vue mounts, so `cachedPublicSettings` is populated on first
 *      render.
 *   2. **Async API** — `App.vue` awaits `appStore.fetchPublicSettings()` on
 *      mount as a fallback (used when injection is missing or stale).
 *
 * If the SSR injection struct forgets to include a feature flag field — the
 * exact bug that hid the "可用渠道" menu after every refresh — the frontend
 * reads `undefined` until the async call resolves. An opt-in flag written as
 * `settings?.xxx_enabled === true` then evaluates to `false` and the menu
 * disappears. An opt-out flag written as `settings?.xxx_enabled !== false`
 * evaluates to `true` (menu stays) but will flicker off if the backend sends
 * `false`.
 *
 * This module hides that `undefined` handling behind two explicit modes.
 *
 * ## Modes
 *
 *   - **`opt-out`** (default enabled) — once settings are loaded, a missing
 *     field remains visible and only an explicit `false` hides it. Use for
 *     features that ship enabled by default (Channel Monitor, Payment).
 *   - **`opt-in`**  (default disabled) — once settings are loaded, a missing
 *     field remains hidden and only an explicit `true` enables it. Use for
 *     features that ship disabled (all staged SubNexus slices).
 *
 * Before a settings response has been successfully loaded, every flag is
 * fail-closed. This is intentional: a stale SSR/cache value must not expose a
 * feature while a refresh is pending or after a failed refresh. SSR injection
 * calls `applySettings`, which marks the payload loaded before the first
 * render, so valid injected configuration is unaffected.
 *
 * For `opt-in` flags to render immediately on refresh, the backend **must**
 * inject the field through `PublicSettingsInjectionPayload`. A drift test in
 * `backend/internal/handler/dto/public_settings_injection_schema_test.go`
 * catches omissions.
 *
 * ## Adding a new flag
 *
 *   1. Backend `service/domain_constants.go`  → `SettingKey<Name>Enabled`
 *   2. Backend `service/settings_view.go`      → `PublicSettings` + `SystemSettings`
 *   3. Backend `service/setting_service.go`    → `GetPublicSettings` / `UpdateSettings` /
 *                                                 `GetAllSettings` / `InitDefaultSettings` /
 *                                                 **`PublicSettingsInjectionPayload`**
 *                                                 (the drift test enforces this)
 *   4. Backend `handler/dto/settings.go`       → `PublicSettings` + `SystemSettings`
 *   5. Backend `handler/setting_handler.go`    → handler response
 *   6. Backend `handler/admin/setting_handler.go` → update request + audit diff
 *   7. Frontend `types/index.ts`               → `PublicSettings` typings
 *   8. Frontend `api/admin/settings.ts`        → admin DTO typings
 *   9. **Frontend `utils/featureFlags.ts` (this file)** → register via `defineFlag`
 *  10. Frontend `views/admin/SettingsView.vue` → Toggle UI + form defaults + save payload
 *  11. Frontend `components/layout/AppSidebar.vue` → attach via `makeSidebarFlag`
 *
 * ## Usage
 *
 * ```ts
 * import { FeatureFlags, makeSidebarFlag } from '@/utils/featureFlags'
 *
 * const flagAvailableChannels = makeSidebarFlag(FeatureFlags.availableChannels)
 * // ...
 * { path: '/available-channels', label: ..., featureFlag: flagAvailableChannels }
 * ```
 *
 * `isFeatureFlagEnabled(flag)` returns the resolved boolean (`true` = show).
 * `makeSidebarFlag(flag)` returns a `() => boolean | undefined` compatible with
 * `AppSidebar.NavItem.featureFlag`, where `false` hides the menu entry.
 */
import { useAppStore } from '@/stores/app'
import type { PublicSettings } from '@/types'
import { DEFAULT_INTERVAL_SECONDS } from '@/constants/channelMonitor'

export type FeatureFlagMode = 'opt-in' | 'opt-out'

export interface FeatureFlagDefinition {
  /** Public-settings key used for lookup. */
  readonly key: keyof PublicSettings
  /** Resolution mode when the key is missing/undefined. */
  readonly mode: FeatureFlagMode
  /** Short human label for logs and debug tooling. */
  readonly label: string
}

function defineFlag<K extends keyof PublicSettings>(
  def: { key: K; mode: FeatureFlagMode; label: string },
): FeatureFlagDefinition {
  return def
}

/**
 * Registered feature flags. Add a new entry here when introducing a new
 * public-settings-driven switch; see the "Adding a new flag" checklist above.
 */
export const FeatureFlags = {
  channelMonitor: defineFlag({
    key: 'channel_monitor_enabled',
    mode: 'opt-out',
    label: 'Channel Monitor',
  }),
  availableChannels: defineFlag({
    key: 'available_channels_enabled',
    mode: 'opt-in',
    label: 'Available Channels',
  }),
  modelPlaza: defineFlag({
    key: 'model_plaza_enabled',
    mode: 'opt-in',
    label: 'Model Plaza',
  }),
  pluginManagement: defineFlag({
    key: 'plugin_management_enabled',
    mode: 'opt-in',
    label: 'Plugin Management',
  }),
  payment: defineFlag({
    key: 'payment_enabled',
    mode: 'opt-out',
    label: 'Payment',
  }),
  riskControl: defineFlag({
    key: 'risk_control_enabled',
    mode: 'opt-in',
    label: 'Risk Control',
  }),
  affiliate: defineFlag({
    key: 'affiliate_enabled',
    mode: 'opt-in',
    label: 'Affiliate',
  }),
  inviteActivities: defineFlag({
    key: 'subnexus_invite_activities_enabled',
    mode: 'opt-in',
    label: 'Invite Activities',
  }),
  activityCenter: defineFlag({
    key: 'subnexus_activity_center_enabled',
    mode: 'opt-in',
    label: 'Activity Center',
  }),
  invoice: defineFlag({
    key: 'invoice_enabled',
    mode: 'opt-in',
    label: 'Invoices',
  }),
  checkIn: defineFlag({
    key: 'subnexus_checkin_enabled',
    mode: 'opt-in',
    label: 'Daily Check-in',
  }),
  leaderboard: defineFlag({
    key: 'subnexus_leaderboard_enabled',
    mode: 'opt-in',
    label: 'Leaderboard',
  }),
  inviteLottery: defineFlag({
    key: 'subnexus_invite_lottery_enabled',
    mode: 'opt-in',
    label: 'Invite Lottery',
  }),
  rechargeWheel: defineFlag({
    key: 'subnexus_recharge_wheel_enabled',
    mode: 'opt-in',
    label: 'Recharge Wheel',
  }),
  inviteMilestone: defineFlag({
    key: 'subnexus_invite_milestone_enabled',
    mode: 'opt-in',
    label: 'Invite Milestone',
  }),
  battlePass: defineFlag({
    key: 'battle_pass_enabled',
    mode: 'opt-in',
    label: 'Battle Pass',
  }),
  marquee: defineFlag({
    key: 'subnexus_marquee_enabled',
    mode: 'opt-in',
    label: 'Broadcast Marquee',
  }),
  firstRecharge: defineFlag({
    key: 'subnexus_first_recharge_enabled',
    mode: 'opt-in',
    label: 'First Recharge Gift',
  }),
  studentRechargeBenefit: defineFlag({
    key: 'subnexus_student_recharge_benefit_enabled',
    mode: 'opt-in',
    label: 'Student Recharge Benefit',
  }),
} as const

export type RegisteredFeatureFlag = keyof typeof FeatureFlags

/**
 * Read the current value of a flag, honoring the mode's fallback.
 * `true`  → the feature is enabled (menu/route should render).
 * `false` → the feature is disabled (menu/route should hide).
 */
export function isFeatureFlagEnabled(flag: FeatureFlagDefinition): boolean {
  const appStore = useAppStore()
  // Never consult a cache that is not backed by a successful current load.
  // This prevents a previously enabled value from leaking through while a
  // refresh is pending or after the public-settings request failed.
  if (appStore.publicSettingsLoaded !== true) return false

  const raw = appStore.cachedPublicSettings?.[flag.key] as
    | boolean
    | undefined
  if (typeof raw === 'boolean') return raw
  // A successfully loaded payload that omits a key still follows the
  // definition's compatibility mode. Staged migration flags are opt-in.
  return flag.mode === 'opt-out'
}

/**
 * Sidebar NavItem.featureFlag accepts a getter that returns
 * `false` to hide. Keeping the same contract lets callers swap in
 * registry-backed flags without changing AppSidebar's filter logic.
 */
export function makeSidebarFlag(flag: FeatureFlagDefinition): () => boolean {
  return () => isFeatureFlagEnabled(flag)
}

/** Activity center is intentionally fail-closed when settings are missing. */
export function isActivityCenterEnabled(): boolean {
  return isFeatureFlagEnabled(FeatureFlags.activityCenter)
}

/** Check-in is intentionally fail-closed when settings are missing. */
export function isCheckInEnabled(): boolean {
  return isFeatureFlagEnabled(FeatureFlags.checkIn)
}

/** Leaderboard is intentionally fail-closed when settings are missing. */
export function isLeaderboardEnabled(): boolean {
  return isFeatureFlagEnabled(FeatureFlags.leaderboard)
}

/** Battle Pass is intentionally fail-closed when settings are missing. */
export function isBattlePassEnabled(): boolean {
  return isFeatureFlagEnabled(FeatureFlags.battlePass)
}

/** Invoice creation is opt-in; history access is handled by the invoice API. */
export function isInvoiceEnabled(): boolean {
  return isFeatureFlagEnabled(FeatureFlags.invoice)
}

/** True when channel monitor feature flag is enabled. */
export function isChannelMonitorRouteEnabled(): boolean {
  return isFeatureFlagEnabled(FeatureFlags.channelMonitor)
}

export type ChannelMonitorMode = 'v1' | 'v2' | 'v3'

/** Exclusive channel-monitor implementation. Invalid/missing -> v1 (opt-in to v2/v3). */
export function getChannelMonitorMode(): ChannelMonitorMode {
  const appStore = useAppStore()
  const mode = appStore.cachedPublicSettings?.channel_monitor_mode
  return mode === 'v2' || mode === 'v3' ? mode : 'v1'
}

export function isChannelMonitorV1Mode(): boolean {
  return isChannelMonitorRouteEnabled() && getChannelMonitorMode() === 'v1'
}

export function isChannelMonitorV2Mode(): boolean {
  return isChannelMonitorRouteEnabled() && getChannelMonitorMode() === 'v2'
}

export function isChannelMonitorV3Mode(): boolean {
  return isChannelMonitorRouteEnabled() && getChannelMonitorMode() === 'v3'
}

export function getChannelMonitorRefreshIntervalSeconds(): number {
  const appStore = useAppStore()
  const configured = appStore.cachedPublicSettings?.channel_monitor_default_interval_seconds
  return configured && configured > 0 ? configured : DEFAULT_INTERVAL_SECONDS
}

/** Hide RPM/TPM on user-facing monitor (scale privacy). Admin always shows full metrics. */
export function isChannelMonitorThroughputHidden(): boolean {
  const appStore = useAppStore()
  return Boolean(appStore.cachedPublicSettings?.channel_monitor_hide_throughput)
}

/**
 * Show quota/balance snapshots on the user-facing monitor page
 * (channel_monitor_show_quota, default off). The backend strips
 * latest_quota server-side when the switch is off; this flag is
 * defense-in-depth only. Admin views always show quota.
 */
export function isChannelMonitorQuotaVisible(): boolean {
  const appStore = useAppStore()
  return appStore.cachedPublicSettings?.channel_monitor_show_quota === true
}
