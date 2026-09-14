import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import PaymentMethodSelector from '@/components/payment/PaymentMethodSelector.vue'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string, fallback?: string) => fallback ?? key,
  }),
}))

describe('PaymentMethodSelector', () => {
  it('selects USDT alongside existing methods and honors payment availability', async () => {
    const methods = [
      { type: 'alipay', display_name: 'Alipay', fee_rate: 0, available: true },
      { type: 'wxpay', display_name: 'WeChat Pay', fee_rate: 0, available: true },
      { type: 'bepusdt', display_name: 'USDT', fee_rate: 0, available: true },
    ]
    const wrapper = mount(PaymentMethodSelector, {
      props: { selected: 'alipay', methods },
    })

    expect(wrapper.findAll('button').map(button => button.text())).toEqual(['Alipay', 'WeChat Pay', 'USDT'])
    await wrapper.get('button[title="USDT"]').trigger('click')
    expect(wrapper.emitted('select')).toEqual([['bepusdt']])

    await wrapper.setProps({
      selected: 'bepusdt',
      methods: methods.map(method => ({ ...method, available: method.type !== 'bepusdt' })),
    })
    expect((wrapper.get('button[title="USDT"]').element as HTMLButtonElement).disabled).toBe(true)
    await wrapper.get('button[title="USDT"]').trigger('click')
    expect(wrapper.emitted('select')).toHaveLength(1)
    wrapper.unmount()
  })

  it('wraps large custom method collections without letting labels widen the selector', () => {
    const methods = Array.from({ length: 12 }, (_, index) => ({
      type: `custom_${index}`,
      display_name: `CUSTOM_PAYMENT_METHOD_${index}`,
      fee_rate: 0,
      available: true,
    }))

    const wrapper = mount(PaymentMethodSelector, {
      props: {
        selected: 'custom_0',
        methods,
      },
    })

    const grid = wrapper.get('[data-testid="payment-method-grid"]')
    expect(grid.classes()).toEqual(expect.arrayContaining(['grid', 'sm:grid-cols-3', 'lg:grid-cols-4']))
    expect(grid.classes()).not.toContain('sm:flex')

    const buttons = wrapper.findAll('button')
    expect(buttons).toHaveLength(methods.length)
    expect(buttons.every(button => button.classes().includes('min-w-0'))).toBe(true)
    expect(buttons.every((button, index) => button.attributes('title') === methods[index].display_name)).toBe(true)
    expect(wrapper.findAll('[data-testid="payment-method-label"]').every(label => label.classes().includes('truncate'))).toBe(true)
  })

  it('shows BSC and TRC20 choices only after selecting USDT', async () => {
    const wrapper = mount(PaymentMethodSelector, {
      props: {
        selected: 'alipay',
        methods: [
          { type: 'alipay', display_name: 'Alipay', fee_rate: 0, available: true },
          { type: 'bepusdt', display_name: 'USDT', fee_rate: 0, available: true },
        ],
        selectedNetwork: 'bepusdt_bep20',
        networkOptions: [
          { type: 'bepusdt_bep20', display_name: 'BSC (BEP20)', fee_rate: 0, available: true },
          { type: 'bepusdt_trc20', display_name: 'TRC20', fee_rate: 0, available: true },
        ],
      },
    })
    expect(wrapper.find('[data-testid="bepusdt-network-selector"]').exists()).toBe(false)
    await wrapper.get('button[title="USDT"]').trigger('click')
    await wrapper.setProps({ selected: 'bepusdt' })
    expect(wrapper.find('[data-testid="bepusdt-network-selector"]').exists()).toBe(true)
    await wrapper.get('[data-testid="bepusdt-network-bepusdt_trc20"]').trigger('click')
    expect(wrapper.emitted('selectNetwork')).toEqual([['bepusdt_trc20']])
  })

  it('shows the configured display name for custom EasyPay methods', () => {
    const wrapper = mount(PaymentMethodSelector, {
      props: {
        selected: 'ldc',
        methods: [{ type: 'ldc', display_name: 'LDC Pay', fee_rate: 0, available: true }],
      },
    })

    expect(wrapper.text()).toContain('LDC Pay')
    expect(wrapper.text()).not.toContain('ldc')
    expect(wrapper.text()).not.toContain('payment.methods.ldc')
  })

  it('uses the generic selected style for custom methods that contain built-in names', () => {
    const wrapper = mount(PaymentMethodSelector, {
      props: {
        selected: 'card_alipay',
        methods: [{ type: 'card_alipay', display_name: 'Card Pay', fee_rate: 0, available: true }],
      },
    })

    const button = wrapper.get('button')
    expect(button.classes()).toContain('border-primary-500')
    expect(button.classes()).not.toContain('border-[#02A9F1]')
  })
})
