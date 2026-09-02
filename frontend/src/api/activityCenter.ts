/**
 * SubNexus activity-center API.
 *
 * This module deliberately models the first migration slice as a
 * custom-only directory of links. It does not expose or couple to any of the
 * legacy reward activities (spin wheels, red-packet rain, invites, or battle
 * pass).
 */
import { apiClient } from './client'

export interface ActivityCenterConfig {
  enabled: boolean
}

export interface ActivityCenterItem {
  id: number
  slug: string
  title: string
  subtitle: string
  description: string
  icon: string
  cover_url: string
  route_path: string
  external_url: string
  action_label: string
  /** The server currently returns `custom` for every item. */
  activity_type: 'custom' | string
  enabled: boolean
  sort_order: number
  start_at?: string | null
  end_at?: string | null
  metadata: Record<string, unknown>
  created_by?: number | null
  created_at: string
  updated_at: string
}

export interface ActivityCenterItemInput {
  slug: string
  title: string
  subtitle?: string
  description?: string
  icon?: string
  cover_url?: string
  route_path?: string
  external_url?: string
  action_label?: string
  /** Optional for backwards-compatible clients; the server defaults to custom. */
  activity_type?: 'custom'
  enabled: boolean
  sort_order: number
  start_at?: string | null
  end_at?: string | null
  metadata?: Record<string, unknown>
}

export interface ActivityCenterListResponse {
  enabled: boolean
  items: ActivityCenterItem[]
}

/** List currently visible custom activity entries for the signed-in user. */
export async function listActivityCenter(): Promise<ActivityCenterListResponse> {
  const { data } = await apiClient.get<ActivityCenterListResponse>('/activity-center')
  return data
}

/** Read the activity-center switch without loading any activity rows. */
export async function getActivityCenterConfig(): Promise<ActivityCenterConfig> {
  const { data } = await apiClient.get<ActivityCenterConfig>('/admin/activity-center/config')
  return data
}

/** Update only the activity-center switch. */
export async function updateActivityCenterConfig(
  payload: ActivityCenterConfig,
): Promise<ActivityCenterConfig> {
  const { data } = await apiClient.put<ActivityCenterConfig>('/admin/activity-center/config', payload)
  return data
}

/** List all custom entries for administrators (only while the switch is on). */
export async function listAdminActivityCenterItems(): Promise<ActivityCenterItem[]> {
  const { data } = await apiClient.get<ActivityCenterItem[]>('/admin/activity-center/items')
  return data
}

export async function createActivityCenterItem(
  payload: ActivityCenterItemInput,
): Promise<ActivityCenterItem> {
  const { data } = await apiClient.post<ActivityCenterItem>('/admin/activity-center/items', payload)
  return data
}

export async function updateActivityCenterItem(
  id: number,
  payload: ActivityCenterItemInput,
): Promise<ActivityCenterItem> {
  const { data } = await apiClient.put<ActivityCenterItem>(`/admin/activity-center/items/${id}`, payload)
  return data
}

export async function deleteActivityCenterItem(id: number): Promise<{ deleted: boolean }> {
  const { data } = await apiClient.delete<{ deleted: boolean }>(`/admin/activity-center/items/${id}`)
  return data
}

const activityCenterAPI = {
  list: listActivityCenter,
  getConfig: getActivityCenterConfig,
  updateConfig: updateActivityCenterConfig,
  listAdmin: listAdminActivityCenterItems,
  create: createActivityCenterItem,
  update: updateActivityCenterItem,
  delete: deleteActivityCenterItem,
}

export default activityCenterAPI
