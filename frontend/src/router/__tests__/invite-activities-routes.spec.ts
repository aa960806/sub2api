import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'

const source = readFileSync(resolve(dirname(fileURLToPath(import.meta.url)), '../index.ts'), 'utf8')

describe('invite activity route integration', () => {
  it.each([
    ['/invite-lottery', 'subnexus_invite_lottery_enabled'],
    ['/recharge-wheel', 'subnexus_recharge_wheel_enabled'],
    ['/invite-milestone', 'subnexus_invite_milestone_enabled'],
  ])('gates %s with its independent public setting', (path, flag) => {
    const routeStart = source.indexOf(`path: '${path}'`)
    expect(routeStart).toBeGreaterThan(-1)
    expect(source.slice(routeStart, routeStart + 500)).toContain(`requiresInviteActivity: '${flag}'`)
  })

  it('checks the aggregate and child flag through the fail-closed helper', () => {
    expect(source).toContain('isInviteActivitySettingsEnabled(')
    expect(source).toContain('to.meta.requiresInviteActivity')
  })

  it('keeps the administrator configuration route independent of user flags', () => {
    const routeStart = source.indexOf("path: '/admin/invite-activities'")
    const block = source.slice(routeStart, routeStart + 500)
    expect(routeStart).toBeGreaterThan(-1)
    expect(block).toContain('requiresAdmin: true')
    expect(block).not.toContain('requiresInviteActivity')
  })
})
