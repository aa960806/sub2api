import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'

const currentDirectory = dirname(fileURLToPath(import.meta.url))
const sourceRoot = resolve(currentDirectory, '../../..')

function read(relativePath: string) {
  return readFileSync(resolve(sourceRoot, relativePath), 'utf8')
}

describe('SubNexus legacy surface scope', () => {
  it('loads compatibility styles through AppLayout', () => {
    const layout = read('components/layout/AppLayout.vue')

    expect(layout).toContain("import '@/styles/subnexus-legacy-surface.css'")
    expect(layout).toContain("'subnexus-legacy-surface': legacySurface")
  })

  it('limits every compatibility selector to opted-in main content', () => {
    const css = read('styles/subnexus-legacy-surface.css')
    const selectorGroups = css
      .split('}')
      .map((rule) => rule.slice(0, rule.indexOf('{')).trim())
      .filter(Boolean)

    expect(selectorGroups.length).toBeGreaterThan(0)
    for (const selectorGroup of selectorGroups) {
      for (const selector of selectorGroup.split(',')) {
        expect(selector.trim()).toContain('.subnexus-legacy-surface main')
      }
    }

    expect(css).not.toMatch(/sidebar|app-header|dropdown/i)
  })

  it('opts in only the retained legacy pages', () => {
    const optedIn = [
      'views/user/ActivityCenterView.vue',
      'views/user/AffiliateView.vue',
      'views/user/BattlePassView.vue',
      'views/user/ChannelStatusV3View.vue',
      'views/user/InviteLotteryView.vue',
      'views/user/InviteMilestoneView.vue',
      'views/user/InvoicesView.vue',
      'views/user/LeaderboardView.vue',
      'views/user/RechargeWheelView.vue',
    ]
    const currentSurface = [
      'views/user/DashboardView.vue',
      'views/user/PaymentView.vue',
      'views/user/ChannelStatusV1View.vue',
      'views/user/ChannelStatusV2View.vue',
    ]

    for (const file of optedIn) {
      expect(read(file), file).toContain('<AppLayout legacy-surface>')
    }
    for (const file of currentSurface) {
      expect(read(file), file).not.toContain('<AppLayout legacy-surface>')
    }
  })

  it('preserves the activity card custom radius', () => {
    const css = read('styles/subnexus-legacy-surface.css')
    const activityCenter = read('views/user/ActivityCenterView.vue')

    expect(css).toContain('.card:not(.ac-card)')
    expect(activityCenter).toMatch(/\.ac-card\s*\{[\s\S]*?border-radius:\s*18px;/)
  })
})
