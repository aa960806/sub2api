import { beforeEach, describe, expect, it, vi } from 'vitest'

const { get, post, put, remove } = vi.hoisted(() => ({
  get: vi.fn(),
  post: vi.fn(),
  put: vi.fn(),
  remove: vi.fn(),
}))

vi.mock('@/api/client', () => ({ apiClient: { get, post, put, delete: remove } }))

import {
  createMarqueeBroadcast,
  deleteMarqueeBroadcast,
  getMarqueeConfig,
  listActiveMarqueeBroadcasts,
  listAdminMarqueeBroadcasts,
  updateMarqueeBroadcast,
  updateMarqueeConfig,
  type MarqueeBroadcastInput,
} from '@/api/marquee'

describe('marquee API', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    get.mockResolvedValue({ data: {} })
    post.mockResolvedValue({ data: {} })
    put.mockResolvedValue({ data: {} })
    remove.mockResolvedValue({ data: { deleted: true } })
  })

  it('uses only the independent marquee endpoints', async () => {
    const input: MarqueeBroadcastInput = { title: '', content: 'Message', enabled: true, priority: 0 }
    await listActiveMarqueeBroadcasts(8)
    await getMarqueeConfig()
    await updateMarqueeConfig({ enabled: true })
    await listAdminMarqueeBroadcasts()
    await createMarqueeBroadcast(input)
    await updateMarqueeBroadcast(4, input)
    await deleteMarqueeBroadcast(4)

    expect(get).toHaveBeenNthCalledWith(1, '/marquee/broadcasts', { params: { limit: 8 } })
    expect(get).toHaveBeenNthCalledWith(2, '/admin/marquee/config')
    expect(put).toHaveBeenNthCalledWith(1, '/admin/marquee/config', { enabled: true })
    expect(get).toHaveBeenNthCalledWith(3, '/admin/marquee/broadcasts')
    expect(post).toHaveBeenCalledWith('/admin/marquee/broadcasts', input)
    expect(put).toHaveBeenNthCalledWith(2, '/admin/marquee/broadcasts/4', input)
    expect(remove).toHaveBeenCalledWith('/admin/marquee/broadcasts/4')
  })
})
