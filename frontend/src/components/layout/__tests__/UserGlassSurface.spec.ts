import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'

const currentDirectory = dirname(fileURLToPath(import.meta.url))
const sourceRoot = resolve(currentDirectory, '../../..')

function read(relativePath: string) {
  return readFileSync(resolve(sourceRoot, relativePath), 'utf8')
}

describe('user rain and glass surface scope', () => {
  it('mounts the shared rain background only for authenticated non-admin routes', () => {
    const layout = read('components/layout/AppLayout.vue')

    expect(layout).toContain("import '@/styles/user-glass-surface.css'")
    expect(layout).toContain("import RainyBackground from '@/components/home/RainyBackground.vue'")
    expect(layout).toContain('route.meta.requiresAuth === true && route.meta.requiresAdmin !== true')
    expect(layout).toContain('<RainyBackground')
    expect(layout).toContain('v-if="isUserSurface && isGlassEnabled"')
    expect(layout).toContain(':droplets-z-index="9"')
    expect(layout).toContain('v-if="!isUserSurface || !isGlassEnabled" class="pointer-events-none fixed inset-0 bg-mesh-gradient"')
  })

  it('keeps the teleported droplets between the wash and existing content layers', () => {
    const layout = read('components/layout/AppLayout.vue')
    const css = read('styles/user-glass-surface.css')
    const droplets = read('components/home/GlassDropletsCanvas.vue')

    // The root must not trap the body-level canvas in a lower stacking context.
    expect(css).not.toContain('isolation: isolate')
    const surfaceRootBlock = css.match(/\.user-glass-surface\s*\{([^}]*)\}/)?.[1] || ''
    expect(surfaceRootBlock).not.toMatch(/z-index\s*:/)
    expect(css).toMatch(/\.user-glass-surface > \.user-surface-content > main[\s\S]*?z-index:\s*10/)
    expect(layout).toContain("{ 'user-surface-content': isUserSurface }")
    expect(droplets).toContain('class="pointer-events-none fixed inset-0 h-full w-full mix-blend-screen"')
    expect(droplets).toContain('zIndex: props.zIndex')
  })

  it('keeps the material selectors rooted in the user surface', () => {
    const css = read('styles/user-glass-surface.css')
    const selectorGroups = (css.replace(/\/\*[\s\S]*?\*\//g, '').match(/[^{}]+(?=\s*\{)/g) || [])
      .map((rule) => rule.trim())
      .filter(Boolean)

    expect(selectorGroups.length).toBeGreaterThan(0)
    for (const selectorGroup of selectorGroups) {
      if (selectorGroup.startsWith('@')) continue
      expect(selectorGroup).toMatch(/\.user-glass-(?:surface|performance-compat)/)
    }

    expect(css).not.toMatch(/(^|[,{\s])body\b/)
    expect(css).not.toMatch(/(^|[,{\s])\.admin\b/)
  })

  it('keeps the shared portal tooltip opt-in and does not alter business controls', () => {
    const header = read('components/layout/AppHeader.vue')
    const layout = read('components/layout/AppLayout.vue')

    expect(header).toContain('user-glass-tooltip')
    expect(layout).not.toContain('user-glass-context')
    expect(layout).not.toContain('document.body.classList')
  })

  it('lets existing dashboard hover utilities win over the inset material rule', () => {
    const css = read('styles/user-glass-surface.css')
    const quickActions = read('components/user/dashboard/UserDashboardQuickActions.vue')
    const recentUsage = read('components/user/dashboard/UserDashboardRecentUsage.vue')

    expect(css).toContain('.user-glass-surface main .user-glass-inset:not(:hover)')
    expect(css).toContain('.user-glass-surface main .user-glass-inset:hover:not([class*=\'hover:bg-\'])')
    expect(css).toContain(":not([class*='dark:hover:bg-'])")
    expect(quickActions).toContain('hover:bg-gray-100')
    expect(recentUsage).toContain('hover:bg-gray-100')
  })

  it('uses opaque themed fills for sticky table cells and preserves row states', () => {
    const css = read('styles/user-glass-surface.css')

    for (const variable of [
      '--user-rain-sticky-col-bg',
      '--user-rain-sticky-col-hover-bg',
      '--user-rain-sticky-col-selected-bg',
      '--user-rain-sticky-header-bg',
      '--user-rain-sticky-header-hover-bg',
    ]) {
      expect(css).toMatch(new RegExp(`${variable}:\\s*rgb\\(`))
    }

    expect(css).toContain('.user-glass-surface main .user-glass-table .sticky-col')
    expect(css).toContain('.user-glass-surface main .user-glass-table tbody tr:hover .sticky-col')
    expect(css).toContain("tbody tr[class*='bg-primary-']")
    expect(css).toContain('.user-glass-surface main .user-glass-table .sticky-header-cell:hover')
  })

  it('keeps the order table on its original structure without an overflow wrapper', () => {
    const orders = read('views/user/UserOrdersView.vue')

    expect(orders).toContain('<OrderTable :orders="orders" :loading="loading">')
    expect(orders).not.toContain('user-glass-panel overflow-hidden rounded-2xl')
    expect(orders).not.toMatch(/<div[^>]*user-glass-panel[^>]*>\s*<OrderTable/)
  })
})
