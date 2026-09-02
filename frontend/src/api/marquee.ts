import { apiClient } from './client'

export interface MarqueeConfig {
  enabled: boolean
}

export interface MarqueeBroadcast {
  id: number
  title: string
  content: string
  source: 'admin'
  enabled: boolean
  priority: number
  start_at?: string | null
  end_at?: string | null
  created_by?: number | null
  created_at: string
  updated_at: string
}

export interface MarqueeBroadcastInput {
  title: string
  content: string
  enabled: boolean
  priority: number
  start_at?: string | null
  end_at?: string | null
}

export interface MarqueeListResponse {
  enabled: boolean
  items: MarqueeBroadcast[]
}

export async function listActiveMarqueeBroadcasts(limit = 12): Promise<MarqueeListResponse> {
  const { data } = await apiClient.get<MarqueeListResponse>('/marquee/broadcasts', {
    params: { limit },
  })
  return data
}

export async function getMarqueeConfig(): Promise<MarqueeConfig> {
  const { data } = await apiClient.get<MarqueeConfig>('/admin/marquee/config')
  return data
}

export async function updateMarqueeConfig(config: MarqueeConfig): Promise<MarqueeConfig> {
  const { data } = await apiClient.put<MarqueeConfig>('/admin/marquee/config', config)
  return data
}

export async function listAdminMarqueeBroadcasts(): Promise<MarqueeBroadcast[]> {
  const { data } = await apiClient.get<MarqueeBroadcast[]>('/admin/marquee/broadcasts')
  return data
}

export async function createMarqueeBroadcast(input: MarqueeBroadcastInput): Promise<MarqueeBroadcast> {
  const { data } = await apiClient.post<MarqueeBroadcast>('/admin/marquee/broadcasts', input)
  return data
}

export async function updateMarqueeBroadcast(id: number, input: MarqueeBroadcastInput): Promise<MarqueeBroadcast> {
  const { data } = await apiClient.put<MarqueeBroadcast>(`/admin/marquee/broadcasts/${id}`, input)
  return data
}

export async function deleteMarqueeBroadcast(id: number): Promise<{ deleted: boolean }> {
  const { data } = await apiClient.delete<{ deleted: boolean }>(`/admin/marquee/broadcasts/${id}`)
  return data
}

const marqueeAPI = {
  listActive: listActiveMarqueeBroadcasts,
  getConfig: getMarqueeConfig,
  updateConfig: updateMarqueeConfig,
  listAdmin: listAdminMarqueeBroadcasts,
  create: createMarqueeBroadcast,
  update: updateMarqueeBroadcast,
  delete: deleteMarqueeBroadcast,
}

export default marqueeAPI
