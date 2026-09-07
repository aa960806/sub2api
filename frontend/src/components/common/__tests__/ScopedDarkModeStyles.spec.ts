import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { compileStyle, parse } from 'vue/compiler-sfc'
import { describe, expect, it } from 'vitest'

const currentDirectory = dirname(fileURLToPath(import.meta.url))
const scopeId = 'data-v-dark-mode-audit'

const cases = [
  { file: '../BroadcastMarquee.vue', selector: '.broadcast-panel' },
  { file: '../CustomerSupportButton.vue', selector: '.support-btn' },
  { file: '../CustomerSupportModal.vue', selector: '.modal-container' },
  { file: '../../../views/user/InviteLotteryView.vue', selector: '.card' },
  { file: '../../../views/user/RechargeWheelView.vue', selector: '.card' },
  { file: '../../../views/user/InviteMilestoneView.vue', selector: '.card' },
  { file: '../../../views/user/PaymentView.vue', selector: '.first-recharge-button.btn-primary' },
]

describe('scoped dark-mode styles', () => {
  it.each(cases)('keeps $file dark styles on the component target', ({ file, selector }) => {
    const filename = resolve(currentDirectory, file)
    const source = readFileSync(filename, 'utf8')
    const { descriptor } = parse(source, { filename })
    const style = descriptor.styles.find((block) => block.scoped)

    expect(style).toBeDefined()
    const result = compileStyle({
      source: style!.content,
      filename,
      id: scopeId,
      scoped: true,
    })

    expect(result.errors).toEqual([])
    expect(result.code).toContain(`html.dark ${selector}[${scopeId}]`)
    expect(result.code).not.toMatch(/(^|})\s*\.dark(?:\s*,\s*\.dark)*\s*\{/m)
  })
})
