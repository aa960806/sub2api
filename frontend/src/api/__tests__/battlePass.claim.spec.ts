import { beforeEach, describe, expect, it, vi } from 'vitest'

const post = vi.hoisted(() => vi.fn())
const put = vi.hoisted(() => vi.fn())
const get = vi.hoisted(() => vi.fn())
vi.mock('../client', () => ({ apiClient: { get, post, put } }))

import {
  claimAllBattlePassRewards,
  claimBattlePassReward,
  completeBattlePassTasksForTest,
  getBattlePassCurrent,
  purchaseBattlePass,
  updateBattlePassSettings,
} from '../battlePass'

describe('battle pass reward claim API', () => {
  beforeEach(() => {
    get.mockReset()
    post.mockReset()
    put.mockReset()
  })

  it('uses the selected reward claim endpoint', async () => {
    post.mockResolvedValue({ data: { claimed_count: 1, rewards: [] } })
    await claimBattlePassReward(42)
    expect(post).toHaveBeenCalledWith('/battle-pass/current/rewards/42/claim')
  })

  it('uses the claim-all endpoint', async () => {
    post.mockResolvedValue({ data: { claimed_count: 2, rewards: [] } })
    await claimAllBattlePassRewards()
    expect(post).toHaveBeenCalledWith('/battle-pass/current/rewards/claim-all')
  })

  it('reads the authoritative user-side gate from the current endpoint', async () => {
    get.mockResolvedValue({ data: { season: null, syncing: false, user_side_enabled: false } })

    await expect(getBattlePassCurrent()).resolves.toEqual({
      season: null,
      syncing: false,
      user_side_enabled: false,
    })
    expect(get).toHaveBeenCalledWith('/battle-pass/current')
  })

  it('sends the same idempotency key in the purchase body and header', async () => {
    post.mockResolvedValue({ data: { id: 9 } })

    await purchaseBattlePass('purchase-key')

    expect(post).toHaveBeenCalledWith(
      '/battle-pass/current/purchase',
      { idempotency_key: 'purchase-key' },
      { headers: { 'Idempotency-Key': 'purchase-key' } },
    )
  })

  it('updates only the explicit admin setting payload', async () => {
    put.mockResolvedValue({ data: { enabled: true } })

    await updateBattlePassSettings({ enabled: true })

    expect(put).toHaveBeenCalledWith('/admin/activity/battle-pass/settings', { enabled: true })
  })

  it('uses the isolated admin test endpoint with an explicit task selector', async () => {
    post.mockResolvedValue({ data: { completed_count: 1, state: {} } })

    await completeBattlePassTasksForTest(12, 34, 56)

    expect(post).toHaveBeenCalledWith('/admin/activity/battle-pass/test/complete', {
      season_id: 12,
      user_id: 34,
      task_id: 56,
    })
  })
})
