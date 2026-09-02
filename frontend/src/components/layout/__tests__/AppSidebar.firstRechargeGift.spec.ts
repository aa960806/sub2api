import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'

const source = readFileSync(resolve(dirname(fileURLToPath(import.meta.url)), '../AppSidebar.vue'), 'utf8')

describe('AppSidebar first recharge gift integration', () => {
  it('keeps the admin entry available while the independent switch is off', () => {
    expect(source).toContain("{ path: '/admin/first-recharge-gift', label: t('nav.firstRechargeGift'), icon: GiftIcon, hideInSimpleMode: true }")
  })
})
