import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import { baseCompile } from '@intlify/message-compiler'

import UserDashboardCheckIn from '../UserDashboardCheckIn.vue'
import zh from '@/i18n/locales/zh'
import en from '@/i18n/locales/en'
import type { CheckInStatus } from '@/api/checkin'

const getCheckInStatus = vi.hoisted(() => vi.fn())
const claimCheckIn = vi.hoisted(() => vi.fn())
const refreshUser = vi.hoisted(() => vi.fn())
const showSuccess = vi.hoisted(() => vi.fn())
const showError = vi.hoisted(() => vi.fn())
const fetchPublicSettings = vi.hoisted(() => vi.fn())

vi.mock('@/api/checkin', () => ({
  getCheckInStatus,
  claimCheckIn,
}))

vi.mock('@/stores', () => ({
  useAppStore: () => ({
    publicSettingsLoaded: true,
    cachedPublicSettings: { subnexus_checkin_enabled: true },
    fetchPublicSettings,
    showSuccess,
    showError,
  }),
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({ refreshUser }),
}))

vi.mock('@/utils/featureFlags', () => ({
  isCheckInEnabled: () => true,
}))

vi.mock('vue-router', () => ({
  useRouter: () => ({ push: vi.fn() }),
}))

function statusFixture(overrides: Partial<CheckInStatus> = {}): CheckInStatus {
  return {
    enabled: true,
    checked_in: false,
    min_amount: 0.01,
    max_amount: 0.1,
    streak: 2,
    next_streak: 3,
    rule_start_day: 3,
    rule_end_day: 3,
    cycle_mode: 'reset',
    milestone_days: 7,
    milestone: false,
    continuous_streak: 9,
    cycle_day: 3,
    cycle_completed_days: 2,
    next_at: '2026-07-27T00:00:00+08:00',
    ...overrides,
  }
}

const testI18n = createI18n({
  legacy: false,
  locale: 'zh',
  fallbackLocale: 'en',
  // The app intentionally uses the runtime-only build; provide its compiler
  // here so this regression test exercises real locale resolution and
  // interpolation instead of mocking `t`.
  messageCompiler: (source) => {
    const { code } = baseCompile(source)
    return new Function(`return ${code}`)() as (context: unknown) => string
  },
  messages: {
    zh: { common: { loading: zh.common.loading, checkin: zh.common.checkin } },
    en: { common: { loading: en.common.loading, checkin: en.common.checkin } },
  },
})

describe('UserDashboardCheckIn translations', () => {
  beforeEach(() => {
    testI18n.global.locale.value = 'zh'
    getCheckInStatus.mockReset()
    claimCheckIn.mockReset()
    refreshUser.mockReset()
    showSuccess.mockReset()
    showError.mockReset()
    fetchPublicSettings.mockReset()
  })

  it('renders Chinese copy and interpolated check-in status without raw keys', async () => {
    const status = statusFixture()
    getCheckInStatus.mockResolvedValue(status)
    claimCheckIn.mockResolvedValue(statusFixture({ checked_in: true, amount: 0.06, continuous_streak: 10, cycle_completed_days: 3 }))
    refreshUser.mockResolvedValue(undefined)

    const wrapper = mount(UserDashboardCheckIn, { global: { plugins: [testI18n] } })
    await flushPromises()

    expect(wrapper.text()).toContain('每日签到')
    expect(wrapper.text()).toContain('连续签到 9 天')
    expect(wrapper.text()).toContain('第 3 天')
    expect(wrapper.text()).toContain('每日签到')
    expect(wrapper.text()).toContain('连续礼盒')
    expect(wrapper.text()).not.toMatch(/checkin\.[a-z]/)

    const claimableDay = wrapper.find('button[title="签到领取第 3 天奖励"]')
    expect(claimableDay.exists()).toBe(true)
    expect(claimableDay.element.disabled).toBe(false)
    await claimableDay.trigger('click')
    await flushPromises()
    expect(claimCheckIn).toHaveBeenCalledTimes(1)
    expect(showSuccess).toHaveBeenCalledWith('签到成功，获得 $0.06')
    expect(wrapper.text()).toContain('今日已签到')
  })

  it('renders English copy after locale switch without raw keys', async () => {
    getCheckInStatus.mockResolvedValue(statusFixture({ continuous_streak: 4, cycle_day: 1, cycle_completed_days: 0 }))

    const wrapper = mount(UserDashboardCheckIn, { global: { plugins: [testI18n] } })
    await flushPromises()
    testI18n.global.locale.value = 'en'
    await wrapper.vm.$nextTick()

    expect(wrapper.text()).toContain('Daily check-in')
    expect(wrapper.text()).toContain('4 consecutive days')
    expect(wrapper.text()).toContain('Day 1')
    expect(wrapper.text()).toContain('Daily check-in')
    expect(wrapper.text()).not.toMatch(/checkin\.[a-z]/)
  })

  it('renders the translated frozen status and amount', async () => {
    getCheckInStatus.mockResolvedValue(statusFixture({ checked_in: true, today_frozen: true, frozen_amount: 1.2 }))

    const wrapper = mount(UserDashboardCheckIn, { global: { plugins: [testI18n] } })
    await flushPromises()

    expect(wrapper.text()).toContain('今日已签到，奖励冻结中')
    expect(wrapper.text()).toContain('已累计冻结 $1.20，充值后自动解锁到余额')
    expect(wrapper.text()).not.toMatch(/checkin\.[a-z]/)
  })
})
