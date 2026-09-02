import { shallowMount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import BattlePassView from '../BattlePassView.vue'

describe('admin battle pass page', () => {
  it('hosts the configuration surface inside the application layout', () => {
    const wrapper = shallowMount(BattlePassView, {
      global: {
        stubs: {
          AppLayout: { template: '<main><slot /></main>' },
          BattlePassConfig: { template: '<section data-testid="battle-pass-config" />' },
        },
      },
    })

    expect(wrapper.get('[data-testid="admin-battle-pass-page"]').exists()).toBe(true)
    expect(wrapper.get('[data-testid="battle-pass-config"]').exists()).toBe(true)
  })
})
