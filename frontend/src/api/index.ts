/**
 * API Client for Sub2API Backend
 * Central export point for all API modules
 */

// Re-export the HTTP client
export { apiClient } from './client'

// Auth API
export { authAPI, isTotp2FARequired, type LoginResponse } from './auth'

// User APIs
export { keysAPI } from './keys'
export { usageAPI } from './usage'
export { userAPI } from './user'
export { redeemAPI, type RedeemHistoryItem } from './redeem'
export { paymentAPI } from './payment'
export { invoicesAPI, shouldShowInvoiceMenu, createInvoiceIdempotencyKey } from './invoices'
export { userGroupsAPI } from './groups'
export { userChannelsAPI } from './channels'
export * as batchImageAPI from './batchImage'
export { totpAPI } from './totp'
export { passkeyAPI, type PasskeyCredentialSummary } from './passkey'
export { default as announcementsAPI } from './announcements'
export { channelMonitorUserAPI } from './channelMonitor'
export { default as activityCenterAPI } from './activityCenter'
export * from './activityCenter'
export { default as checkInAPI } from './checkin'
export * from './checkin'
export { default as leaderboardAPI } from './leaderboard'
export * from './leaderboard'
export { default as inviteActivitiesAPI } from './inviteActivities'
export * from './inviteActivities'
export { default as marqueeAPI } from './marquee'
export * from './marquee'
export * from './battlePass'
export * from './studentRechargeBenefit'

// Admin APIs
export { adminAPI } from './admin'

// Default export
export { default } from './client'
