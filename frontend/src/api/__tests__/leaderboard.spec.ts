import { beforeEach, describe, expect, it, vi } from 'vitest'

const { get, put } = vi.hoisted(() => ({ get: vi.fn(), put: vi.fn() }))

vi.mock('@/api/client', () => ({
  apiClient: { get, put },
}))

import {
  getInviteLeaderboard,
  getLeaderboard,
  getLeaderboardConfig,
  updateLeaderboardConfig,
} from '@/api/leaderboard'

describe('leaderboard API', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    get.mockResolvedValue({ data: {} })
    put.mockResolvedValue({ data: {} })
  })

  it('uses isolated read and configuration endpoints with window/limit params', async () => {
    await getLeaderboard('month', 12)
    await getInviteLeaderboard('today', 8)
    await getLeaderboardConfig()
    const config = { enabled: false, weekly_enabled: false, weekly_top_n: 3, weekly_reward: 1, weekly_rewards: [1, 1, 1], monthly_enabled: false, monthly_top_n: 3, monthly_reward: 5, monthly_rewards: [5, 5, 5] }
    await updateLeaderboardConfig(config)

    expect(get).toHaveBeenNthCalledWith(1, '/activity/leaderboard', { params: { window: 'month', limit: 12 } })
    expect(get).toHaveBeenNthCalledWith(2, '/activity/invite-leaderboard', { params: { window: 'today', limit: 8 } })
    expect(get).toHaveBeenNthCalledWith(3, '/admin/leaderboard/config')
    expect(put).toHaveBeenCalledWith('/admin/leaderboard/config', config)
  })
})
