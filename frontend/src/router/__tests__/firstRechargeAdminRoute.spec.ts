import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'

const source = readFileSync(resolve(dirname(fileURLToPath(import.meta.url)), '../index.ts'), 'utf8')

describe('first recharge gift admin route', () => {
  it('keeps an admin-only configuration route independent from the user rollout switch', () => {
    expect(source).toContain("path: '/admin/first-recharge-gift'")
    expect(source).toContain("name: 'AdminFirstRechargeGift'")
    expect(source).toContain("component: () => import('@/views/admin/FirstRechargeGiftView.vue')")
    expect(source).not.toMatch(/path: '\/admin\/first-recharge-gift'[\s\S]{0,500}requiresPayment: true/)
    expect(source).toMatch(/restrictedPaths = \[[\s\S]*'\/admin\/first-recharge-gift'/)
  })
})
