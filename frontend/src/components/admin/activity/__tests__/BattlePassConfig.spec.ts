import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import BattlePassConfig from '../BattlePassConfig.vue'

const getSettings = vi.hoisted(() => vi.fn())
const updateSettings = vi.hoisted(() => vi.fn())
const listSeasons = vi.hoisted(() => vi.fn())
const getSeason = vi.hoisted(() => vi.fn())
const createSeason = vi.hoisted(() => vi.fn())
const updateSeason = vi.hoisted(() => vi.fn())
const validateSeason = vi.hoisted(() => vi.fn())
const publishSeason = vi.hoisted(() => vi.fn())
const getTestState = vi.hoisted(() => vi.fn())
const activateTestSeason = vi.hoisted(() => vi.fn())
const completeTestTasks = vi.hoisted(() => vi.fn())
const getAllGroups = vi.hoisted(() => vi.fn())
const appStore = vi.hoisted(() => ({
  cachedPublicSettings: { battle_pass_enabled: false } as null | { battle_pass_enabled?: boolean },
}))

vi.mock('@/api/battlePass', () => ({
  getBattlePassSettings: (...args: unknown[]) => getSettings(...args),
  updateBattlePassSettings: (...args: unknown[]) => updateSettings(...args),
  listBattlePassSeasons: (...args: unknown[]) => listSeasons(...args),
  getBattlePassSeason: (...args: unknown[]) => getSeason(...args),
  createBattlePassSeason: (...args: unknown[]) => createSeason(...args),
  updateBattlePassSeason: (...args: unknown[]) => updateSeason(...args),
  validateBattlePassSeason: (...args: unknown[]) => validateSeason(...args),
  publishBattlePassSeason: (...args: unknown[]) => publishSeason(...args),
  getBattlePassTestState: (...args: unknown[]) => getTestState(...args),
  activateBattlePassSeasonForTest: (...args: unknown[]) => activateTestSeason(...args),
  completeBattlePassTasksForTest: (...args: unknown[]) => completeTestTasks(...args),
  pauseBattlePassSeason: vi.fn(),
  resumeBattlePassSeason: vi.fn(),
  endBattlePassSeason: vi.fn(),
}))
vi.mock('@/api/admin/groups', () => ({
  getAll: (...args: unknown[]) => getAllGroups(...args),
  groupsAPI: { getAll: (...args: unknown[]) => getAllGroups(...args) },
  default: { getAll: (...args: unknown[]) => getAllGroups(...args) },
}))
vi.mock('@/stores/app', () => ({ useAppStore: () => appStore }))

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return { ...actual, useI18n: () => ({ t: (key: string) => key }) }
})

vi.mock('@/composables/useStepUp', () => ({
  useStepUp: () => ({ run: async (fn: () => unknown) => fn() }),
}))

describe('BattlePassConfig', () => {
  beforeEach(() => {
    getSettings.mockReset()
    updateSettings.mockReset()
    listSeasons.mockReset()
    getSeason.mockReset()
    createSeason.mockReset()
    updateSeason.mockReset()
    validateSeason.mockReset()
    publishSeason.mockReset()
    getTestState.mockReset()
    activateTestSeason.mockReset()
    completeTestTasks.mockReset()
    getAllGroups.mockReset()
    appStore.cachedPublicSettings = { battle_pass_enabled: false }
    getSettings.mockResolvedValue({ enabled: false })
    listSeasons.mockResolvedValue([])
    getAllGroups.mockResolvedValue([])
  })

  it('loads disabled by default', async () => {
    const wrapper = mount(BattlePassConfig, {
      global: { stubs: { TotpStepUpDialog: true } },
    })
    await flushPromises()
    expect(getSettings).toHaveBeenCalled()
    expect(listSeasons).toHaveBeenCalled()
    expect((wrapper.find('input[type="checkbox"]').element as HTMLInputElement).checked).toBe(false)
  })

  it('fails closed when settings cannot be loaded', async () => {
    getSettings.mockRejectedValue(new Error('settings unavailable'))

    const wrapper = mount(BattlePassConfig, {
      global: { stubs: { TotpStepUpDialog: true } },
    })
    await flushPromises()

    expect((wrapper.find('input[type="checkbox"]').element as HTMLInputElement).checked).toBe(false)
    expect(wrapper.text()).toContain('settings unavailable')
  })

  it('restores the last confirmed disabled state when enabling fails', async () => {
    updateSettings.mockRejectedValue(new Error('save failed'))
    const wrapper = mount(BattlePassConfig, {
      global: { stubs: { TotpStepUpDialog: true } },
    })
    await flushPromises()

    const toggle = wrapper.find('input[type="checkbox"]')
    await toggle.setValue(true)
    await flushPromises()

    expect(updateSettings).toHaveBeenCalledWith({ enabled: true })
    expect((toggle.element as HTMLInputElement).checked).toBe(false)
    expect(wrapper.text()).toContain('save failed')
  })

  it('synchronizes the public opt-in flag only after the backend confirms enabling', async () => {
    updateSettings.mockResolvedValue({ enabled: true })
    const wrapper = mount(BattlePassConfig, {
      global: { stubs: { TotpStepUpDialog: true } },
    })
    await flushPromises()

    await wrapper.find('input[type="checkbox"]').setValue(true)
    await flushPromises()

    expect(appStore.cachedPublicSettings?.battle_pass_enabled).toBe(true)
  })

  it('supports adding and removing levels, tasks, and rewards with minimum guards', async () => {
    const wrapper = mount(BattlePassConfig, {
      global: { stubs: { TotpStepUpDialog: true } },
    })
    await flushPromises()

    await wrapper.find('button[title="新增等级"]').trigger('click')
    expect(wrapper.findAll('button[title^="删除等级"]').length).toBe(2)
    await wrapper.find('button[title="删除等级 2"]').trigger('click')
    expect(wrapper.findAll('button[aria-label^="删除等级"]').length).toBe(1)
    expect(wrapper.find('button[title="至少保留一个等级"]').exists()).toBe(true)

    await wrapper.find('button[title="新增任务"]').trigger('click')
    expect(wrapper.findAll('button[title="删除任务"]').length).toBe(2)
    await wrapper.findAll('button[title="删除任务"]')[1].trigger('click')
    expect(wrapper.findAll('button[aria-label^="删除任务"]').length).toBe(1)
    expect(wrapper.find('button[title="至少保留一个任务"]').exists()).toBe(true)

    await wrapper.find('button[title="新增奖励"]').trigger('click')
    expect(wrapper.findAll('button[aria-label^="删除奖励"]').length).toBe(3)
    await wrapper.findAll('button[aria-label^="删除奖励"]')[2].trigger('click')
    expect(wrapper.findAll('button[aria-label^="删除奖励"]').length).toBe(2)
  })

  it('fills all implemented task types without deferred labels', async () => {
    const wrapper = mount(BattlePassConfig, {
      global: { stubs: { TotpStepUpDialog: true } },
    })
    await flushPromises()

    await wrapper.get('[data-testid="battle-pass-fill-all-tasks"]').trigger('click')

    const taskTypes = wrapper.findAll('[data-testid="battle-pass-task-type"]')
      .map((item) => (item.element as HTMLSelectElement).value)
    expect(taskTypes).toEqual([
      'request_count', 'cost_amount', 'active_days', 'distinct_model_families', 'image_count',
      'video_count', 'recharge_count', 'recharge_amount', 'valid_invite_count', 'invitee_recharge_count',
    ])
    expect(wrapper.text()).not.toContain('暂不可发布')
  })

  it('selects subscription rewards by active subscription group name and explains cosmetic effects', async () => {
    getAllGroups.mockResolvedValue([
      { id: 7, name: 'OpenAI 月卡', platform: 'openai', status: 'active', subscription_type: 'subscription' },
      { id: 8, name: '普通计费组', platform: 'openai', status: 'active', subscription_type: 'standard' },
      { id: 9, name: '停用订阅组', platform: 'anthropic', status: 'inactive', subscription_type: 'subscription' },
    ])
    const wrapper = mount(BattlePassConfig, {
      global: { stubs: { TotpStepUpDialog: true } },
    })
    await flushPromises()

    await wrapper.findAll('[data-testid="battle-pass-reward-type"]')[0].setValue('subscription_days')
    await flushPromises()

    const groupSelect = wrapper.get('[data-testid="battle-pass-subscription-group"]')
    expect((groupSelect.element as HTMLSelectElement).value).toBe('7')
    expect(groupSelect.text()).toContain('OpenAI 月卡 (#7) · openai')
    expect(groupSelect.text()).not.toContain('普通计费组')
    expect(groupSelect.text()).not.toContain('停用订阅组')
    expect(wrapper.text()).toContain('称号与徽章领取后会出现在用户战令')
    expect(wrapper.text()).toContain('普通计费分组不会出现在列表中')
  })

  it('blocks an incomplete draft locally with a concrete field message', async () => {
    const wrapper = mount(BattlePassConfig, {
      global: { stubs: { TotpStepUpDialog: true } },
    })
    await flushPromises()

    await wrapper.get('[data-testid="battle-pass-save-draft"]').trigger('click')

    expect(createSeason).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('请填写赛季名称')
  })

  it('renders the structured API message instead of object Object', async () => {
    createSeason.mockRejectedValue({ code: 'BATTLE_PASS_SEASON_INVALID', message: 'timezone is invalid' })
    const wrapper = mount(BattlePassConfig, {
      global: { stubs: { TotpStepUpDialog: true } },
    })
    await flushPromises()
    await wrapper.get('input[aria-label="赛季名称"]').setValue('第二赛季')

    await wrapper.get('[data-testid="battle-pass-save-draft"]').trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('赛季时区无效，请填写例如 Asia/Shanghai')
    expect(wrapper.text()).not.toContain('[object Object]')
  })

  it('enables validation and publishing after a draft is created', async () => {
    const saved = {
      id: 10,
      name: '第二赛季',
      description: '',
      status: 'draft',
      runtime_status: 'draft',
      timezone: 'Asia/Shanghai',
      start_at: '2026-09-02T00:00:00Z',
      end_at: '2026-10-02T00:00:00Z',
      premium_price: 9.9,
      max_level: 1,
      user_side_enabled: true,
    }
    createSeason.mockResolvedValue(saved)
    const wrapper = mount(BattlePassConfig, {
      global: { stubs: { TotpStepUpDialog: true } },
    })
    await flushPromises()
    listSeasons.mockResolvedValue([saved])
    await wrapper.get('input[aria-label="赛季名称"]').setValue(saved.name)

    await wrapper.get('[data-testid="battle-pass-save-draft"]').trigger('click')
    await flushPromises()

    expect(createSeason).toHaveBeenCalledOnce()
    expect(wrapper.get('[data-testid="battle-pass-validate-draft"]').attributes('disabled')).toBeUndefined()
    expect(wrapper.get('[data-testid="battle-pass-publish-season"]').attributes('disabled')).toBeUndefined()
    expect(wrapper.text()).toContain('草稿已保存')
  })

  it('makes published seasons read-only while retaining state controls', async () => {
    const season = {
      id: 9,
      name: '已发布赛季',
      description: '',
      status: 'scheduled',
      runtime_status: 'active',
      timezone: 'Asia/Shanghai',
      start_at: '2026-09-01T00:00:00Z',
      end_at: '2026-09-08T00:00:00Z',
      premium_price: 1,
      max_level: 1,
      user_side_enabled: true,
    }
    listSeasons.mockResolvedValue([season])
    getSeason.mockResolvedValue({
      ...season,
      levels: [{ level: 1, required_exp: 0 }],
      tasks: [{ name: '请求', description: '', task_type: 'request_count', period_type: 'daily', target_value: 1, exp_reward: 1, filter_scope: 'all', filter_values: [], display_order: 0, enabled: true }],
      rewards: [{ level: 1, track: 'free', reward_type: 'balance', payload: { amount: 1 } }, { level: 1, track: 'premium', reward_type: 'balance', payload: { amount: 2 } }],
    })
    const wrapper = mount(BattlePassConfig, {
      global: { stubs: { TotpStepUpDialog: true } },
    })
    await flushPromises()
    const seasonButton = wrapper.findAll('button').find((button) => button.text().includes('已发布赛季'))
    expect(seasonButton).toBeDefined()
    await seasonButton!.trigger('click')
    await flushPromises()

    expect(getSeason).toHaveBeenCalledWith(9)
    expect(wrapper.find('fieldset').attributes('disabled')).toBeDefined()
    expect((wrapper.find('button[title="新增等级"]').element as HTMLButtonElement).disabled).toBe(true)
    expect(wrapper.findAll('button').some((button) => button.text().includes('暂停赛季'))).toBe(true)
  })

  it('shows local test tools only when the backend enables them', async () => {
    const season = {
      id: 12,
      name: '验收赛季',
      description: '',
      status: 'scheduled',
      runtime_status: 'active',
      timezone: 'Asia/Shanghai',
      start_at: '2026-09-01T00:00:00Z',
      end_at: '2026-09-08T00:00:00Z',
      premium_price: 1,
      max_level: 1,
      user_side_enabled: true,
    }
    getSettings.mockResolvedValue({ enabled: true, test_tools_enabled: true })
    listSeasons.mockResolvedValue([season])
    getSeason.mockResolvedValue({
      ...season,
      levels: [{ level: 1, required_exp: 0 }],
      tasks: [{ id: 91, name: '请求', description: '', task_type: 'request_count', period_type: 'daily', target_value: 1, exp_reward: 10, filter_scope: 'all', filter_values: [], display_order: 0, enabled: true }],
      rewards: [{ id: 92, level: 1, track: 'free', reward_type: 'balance', payload: { amount: 1 } }],
    })
    getTestState.mockResolvedValue({
      season,
      user: { id: 1, email: 'admin@example.com' },
      progress: { exp: 0, level: 1, level_start_exp: 0, next_level_exp: null, premium_unlocked: false, updated_at: season.start_at },
      tasks: [{ id: 91, name: '请求', description: '', task_type: 'request_count', period_type: 'daily', target_value: 1, exp_reward: 10, filter_scope: 'all', filter_values: [], display_order: 0, enabled: true, current_value: 0, completed: false, period_key: '2026-09-01' }],
      rewards: [],
    })
    completeTestTasks.mockResolvedValue({ completed_count: 1, state: { ...(await getTestState()), tasks: [{ ...(await getTestState()).tasks[0], current_value: 1, completed: true }] } })

    const wrapper = mount(BattlePassConfig, {
      global: { stubs: { TotpStepUpDialog: true } },
    })
    await flushPromises()
    const seasonButton = wrapper.findAll('button').find((button) => button.text().includes('验收赛季'))
    await seasonButton!.trigger('click')
    await flushPromises()

    expect(wrapper.find('[data-testid="battle-pass-test-tools"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('admin@example.com')
    await wrapper.get('[data-testid="battle-pass-test-complete-all"]').trigger('click')
    await flushPromises()
    expect(completeTestTasks).toHaveBeenCalledWith(12, 1, 0)
  })
})
