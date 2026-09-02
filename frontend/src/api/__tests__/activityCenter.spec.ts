import { beforeEach, describe, expect, it, vi } from 'vitest'

const { get, post, put, remove } = vi.hoisted(() => ({
  get: vi.fn(),
  post: vi.fn(),
  put: vi.fn(),
  remove: vi.fn(),
}))

vi.mock('@/api/client', () => ({
  apiClient: { get, post, put, delete: remove },
}))

import {
  createActivityCenterItem,
  deleteActivityCenterItem,
  getActivityCenterConfig,
  listActivityCenter,
  listAdminActivityCenterItems,
  updateActivityCenterConfig,
  updateActivityCenterItem,
  type ActivityCenterItemInput,
} from '@/api/activityCenter'

describe('activity center API', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    get.mockResolvedValue({ data: {} })
    post.mockResolvedValue({ data: {} })
    put.mockResolvedValue({ data: {} })
    remove.mockResolvedValue({ data: { deleted: true } })
  })

  it('uses the authenticated user and admin endpoints', async () => {
    const input: ActivityCenterItemInput = {
      slug: 'custom-entry',
      title: 'Custom entry',
      activity_type: 'custom',
      enabled: false,
      sort_order: 0,
    }

    await listActivityCenter()
    await getActivityCenterConfig()
    await updateActivityCenterConfig({ enabled: true })
    await listAdminActivityCenterItems()
    await createActivityCenterItem(input)
    await updateActivityCenterItem(7, input)
    await deleteActivityCenterItem(7)

    expect(get).toHaveBeenNthCalledWith(1, '/activity-center')
    expect(get).toHaveBeenNthCalledWith(2, '/admin/activity-center/config')
    expect(put).toHaveBeenNthCalledWith(1, '/admin/activity-center/config', { enabled: true })
    expect(get).toHaveBeenNthCalledWith(3, '/admin/activity-center/items')
    expect(post).toHaveBeenCalledWith('/admin/activity-center/items', input)
    expect(put).toHaveBeenNthCalledWith(2, '/admin/activity-center/items/7', input)
    expect(remove).toHaveBeenCalledWith('/admin/activity-center/items/7')
  })
})
