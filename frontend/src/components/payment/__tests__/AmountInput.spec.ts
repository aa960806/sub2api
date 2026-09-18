import { afterEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, mount } from '@vue/test-utils'
import { defineComponent, ref } from 'vue'
import AmountInput from '@/components/payment/AmountInput.vue'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => key,
  }),
}))

enableAutoUnmount(afterEach)

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

function mountInput(value: number | null = null) {
  return mount(AmountInput, { props: { modelValue: value, amounts: [] } })
}

describe('recharge amount input', () => {
  it.each(['10abc', '10.555', '-10', '1e2'])('restores the accepted amount after rejecting %s', async (value) => {
    const wrapper = mountInput(10)
    const input = wrapper.get('input')
    await input.setValue(value)
    expect((input.element as HTMLInputElement).value).toBe('10')
    expect(wrapper.emitted('update:modelValue')).toBeUndefined()
  })

  it('restores the last typed amount rather than a stale prop', async () => {
    const wrapper = mountInput()
    const input = wrapper.get('input')
    await input.setValue('12.50')
    await input.setValue('12.500')
    expect((input.element as HTMLInputElement).value).toBe('12.50')
    expect(wrapper.emitted('update:modelValue')).toEqual([[12.5]])
  })

  it('preserves decimal editing and allows clearing the amount', async () => {
    const wrapper = mountInput()
    const input = wrapper.get('input')
    for (const value of ['0', '0.', '0.5', '0.50', '']) await input.setValue(value)
    expect(wrapper.emitted('update:modelValue')).toEqual([[null], [null], [0.5], [0.5], [null]])
    expect((input.element as HTMLInputElement).value).toBe('')
  })
})
