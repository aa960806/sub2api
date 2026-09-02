import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'

const source = readFileSync(resolve(dirname(fileURLToPath(import.meta.url)), '../AppSidebar.vue'), 'utf8')

describe('AppSidebar invite activity integration', () => {
  it.each([
    ['/invite-lottery', 'flagInviteLottery'],
    ['/recharge-wheel', 'flagRechargeWheel'],
    ['/invite-milestone', 'flagInviteMilestone'],
  ])('hides %s through its composite fail-closed flag', (path, flag) => {
    const itemStart = source.indexOf(`{ path: '${path}'`)
    expect(itemStart).toBeGreaterThan(-1)
    expect(source.slice(itemStart, itemStart + 260)).toContain(`featureFlag: ${flag}`)
  })

  it('keeps the admin settings entry available before rollout', () => {
    expect(source).toContain("{ path: '/admin/invite-activities', label: t('inviteActivities.admin.title'), icon: GiftIcon, hideInSimpleMode: true }")
  })
})
