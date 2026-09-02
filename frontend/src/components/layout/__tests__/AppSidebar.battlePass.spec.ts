import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'

const source = readFileSync(resolve(dirname(fileURLToPath(import.meta.url)), '../AppSidebar.vue'), 'utf8')

describe('AppSidebar Battle Pass integration', () => {
  it('gates the user entry with the opt-in flag', () => {
    expect(source).toContain('const flagBattlePass = makeSidebarFlag(FeatureFlags.battlePass)')
    expect(source).toContain("{ path: '/battle-pass', label: t('battlePass.title'), icon: TicketIcon, hideInSimpleMode: true, featureFlag: flagBattlePass }")
  })

  it('keeps the admin configuration entry available while the user flag is off', () => {
    expect(source).toContain("{ path: '/admin/battle-pass', label: t('battlePass.adminTitle'), icon: TicketIcon, hideInSimpleMode: true }")
  })
})
