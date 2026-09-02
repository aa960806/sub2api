/**
 * Type definitions for Vue Router meta fields
 * Extends the RouteMeta interface with custom properties
 */

import 'vue-router'

declare module 'vue-router' {
  interface RouteMeta {
    /**
     * Whether this route requires authentication
     * @default true
     */
    requiresAuth?: boolean

    /**
     * Whether this route requires admin role
     * @default false
     */
    requiresAdmin?: boolean

    /**
     * Page title for this route
     */
    title?: string

    /**
     * Optional breadcrumb items for navigation
     */
    breadcrumbs?: Array<{
      label: string
      to?: string
    }>

    /**
     * Icon name for this route (for sidebar navigation)
     */
    icon?: string

    /**
     * Whether to hide this route from navigation menu
     * @default false
     */
    hideInMenu?: boolean

    /**
     * Whether this route requires internal payment system to be enabled
     * @default false
     */
    requiresPayment?: boolean

    /**
     * 是否要求风控中心功能开关已启用
     * @default false
     */
    requiresRiskControl?: boolean

    /**
     * Whether the user invoice route requires the invoice feature gate.
     * Administrators use the management route even while new applications
     * are disabled so historical requests remain operable.
     */
    requiresInvoice?: boolean

    /**
     * Whether this route requires the SubNexus activity-center opt-in flag.
     * Missing settings and failed settings loads are treated as disabled.
     */
    requiresActivityCenter?: boolean

    /** Whether this route requires the SubNexus leaderboard opt-in flag. */
    requiresLeaderboard?: boolean

    /** Whether this route requires the independent Battle Pass opt-in flag. */
    requiresBattlePass?: boolean

    /**
     * Independent public flag for one migrated invite activity. The route
     * also requires the aggregate invite-activities switch.
     */
    requiresInviteActivity?:
      | 'subnexus_invite_lottery_enabled'
      | 'subnexus_recharge_wheel_enabled'
      | 'subnexus_invite_milestone_enabled'

    /**
     * i18n key for the page title
     */
    titleKey?: string

    /**
     * i18n key for the page description
     */
    descriptionKey?: string
  }
}
