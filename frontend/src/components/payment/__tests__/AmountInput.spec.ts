import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { defineComponent, ref } from 'vue'
import AmountInput from '@/components/payment/AmountInput.vue'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => key,
  }),
}))

function mountAmountInput(initialValue: number | null, amounts = [50, 100, 250]) {
  const Host = defineComponent({
    components: { AmountInput },
    setup() {
      return {
        amount: ref<number | null>(initialValue),
        amounts,
      }
    },
    template: '<AmountInput v-model="amount" :amounts="amounts" />',
  })

  return mount(Host)
}

describe('AmountInput', () => {
  it('selects the first available amount by default and displays it in the input', async () => {
    const wrapper = mountAmountInput(null)
    await wrapper.vm.$nextTick()

    expect(wrapper.get<HTMLInputElement>('[data-testid="recharge-amount-input"]').element.value).toBe('50')
    expect(wrapper.get('[data-amount="50"]').attributes('aria-pressed')).toBe('true')
  })

  it('copies a clicked preset into the input and selects only that preset', async () => {
    const wrapper = mountAmountInput(50)

    await wrapper.get('[data-amount="100"]').trigger('click')
    await wrapper.vm.$nextTick()

    expect(wrapper.get<HTMLInputElement>('[data-testid="recharge-amount-input"]').element.value).toBe('100')
    expect(wrapper.get('[data-amount="50"]').attributes('aria-pressed')).toBe('false')
    expect(wrapper.get('[data-amount="100"]').attributes('aria-pressed')).toBe('true')
  })

  it('selects a matching preset for typed input and clears the selection for a custom amount', async () => {
    const wrapper = mountAmountInput(50)
    const input = wrapper.get('[data-testid="recharge-amount-input"]')

    await input.setValue('250.00')
    await wrapper.vm.$nextTick()
    expect(wrapper.get('[data-amount="250"]').attributes('aria-pressed')).toBe('true')

    await input.setValue('275.50')
    await wrapper.vm.$nextTick()
    expect(wrapper.findAll('[data-amount]').every(button => button.attributes('aria-pressed') === 'false')).toBe(true)
    expect(wrapper.get<HTMLInputElement>('[data-testid="recharge-amount-input"]').element.value).toBe('275.50')
  })

  it('does not restore the default after the user clears the input', async () => {
    const wrapper = mountAmountInput(null)
    await wrapper.vm.$nextTick()

    const input = wrapper.get('[data-testid="recharge-amount-input"]')
    await input.setValue('')
    await wrapper.vm.$nextTick()

    expect(wrapper.get<HTMLInputElement>('[data-testid="recharge-amount-input"]').element.value).toBe('')
    expect(wrapper.findAll('[data-amount]').every(button => button.attributes('aria-pressed') === 'false')).toBe(true)
  })
})
