import { beforeEach, describe, expect, it, vi } from 'vitest'

const { get, put } = vi.hoisted(() => ({ get: vi.fn(), put: vi.fn() }))

vi.mock('@/api/client', () => ({
  apiClient: { get, put },
}))

import { adminPaymentAPI, type AdminFirstRechargeGiftConfig } from '@/api/admin/payment'

describe('admin first recharge gift API', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    get.mockResolvedValue({ data: {} })
    put.mockResolvedValue({ data: {} })
  })

  it('uses the isolated admin configuration endpoints', async () => {
    const config: AdminFirstRechargeGiftConfig = {
      enabled: false,
      price: 9.9,
      credited_amount: 12,
      ratio: 1.2121,
    }

    await adminPaymentAPI.getFirstRechargeGiftConfig()
    await adminPaymentAPI.updateFirstRechargeGiftConfig(config)

    expect(get).toHaveBeenCalledWith('/admin/payment/first-recharge-gift/config')
    expect(put).toHaveBeenCalledWith('/admin/payment/first-recharge-gift/config', config)
  })
})
