import { defineComponent } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import ModelEvaluationsView from '../ModelEvaluationsView.vue'
const api = vi.hoisted(() => ({ config: vi.fn(), setConfig: vi.fn(), tasks: vi.fn(), cleanup: vi.fn(), deleteTask: vi.fn(), run: vi.fn() }))
const store = vi.hoisted(() => ({ showSuccess: vi.fn(), showError: vi.fn(), fetchPublicSettings: vi.fn() }))
vi.mock('@/api/modelEvaluations', () => ({ adminModelEvaluationsAPI: api }))
vi.mock('@/api/admin/groups', () => ({ getAllIncludingInactive: vi.fn(async () => [{ id: 3, name: 'Group' }]) }))
vi.mock('@/stores/app', () => ({ useAppStore: () => store }))
vi.mock('@/components/layout/AppLayout.vue', () => ({ default: { template: '<div><slot /></div>' } }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key, locale: { value: 'en' } }) }))
vi.mock('@/components/model-evaluation/ModelEvaluationGallery.vue', () => ({ default: { template: '<div />' } }))
vi.mock('@/components/model-evaluation/ModelEvaluationTaskDialog.vue', () => ({ default: { template: '<div />' } }))
const Dialog = defineComponent({ props: ['show'], template: '<div v-if="show" data-testid="dialog"><slot /><slot name="footer" /></div>' })
const task = { id: 9, name: 'Test', group_id: 3, group_name: 'Group', model: 'test-model', enabled: true, endpoint: 'https://example.com/v1/messages', interval_seconds: 60, retention_days: 7, max_records: 50 }
function render() { return mount(ModelEvaluationsView, { global: { stubs: { AppLayout: { template: '<div><slot /></div>' }, BaseDialog: Dialog } } }) }
function findButton(wrapper: ReturnType<typeof render>, label: string) { return wrapper.findAll('button').find(button => button.text() === label)! }
describe('model evaluation admin controls', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    api.config.mockResolvedValue({ enabled: false })
    api.tasks.mockResolvedValue({ items: [task] })
    api.setConfig.mockResolvedValue({ enabled: true })
    api.cleanup.mockResolvedValue({ deleted: 4 })
  })
  it('keeps maintenance available when disabled and refreshes public settings after enabling', async () => {
    const wrapper = render()
    await flushPromises()
    expect(findButton(wrapper, 'modelEvaluations.admin.run').attributes('disabled')).toBeDefined()
    expect(findButton(wrapper, 'modelEvaluations.admin.cleanup').attributes('disabled')).toBeUndefined()
    await wrapper.find('[data-testid="monitor-enabled"]').setValue(true)
    await flushPromises()
    expect(api.setConfig).toHaveBeenCalledWith(true)
    expect(store.fetchPublicSettings).toHaveBeenCalledWith(true)
    expect(findButton(wrapper, 'modelEvaluations.admin.run').attributes('disabled')).toBeUndefined()
    wrapper.unmount()
  })
  it('defaults cleanup to retention rules and requires explicit selection for full removal', async () => {
    const wrapper = render()
    await flushPromises()
    await findButton(wrapper, 'modelEvaluations.admin.cleanup').trigger('click')
    expect(api.cleanup).not.toHaveBeenCalled()
    await wrapper.find('[data-testid="dialog"] select').setValue(9)
    await findButton(wrapper, 'modelEvaluations.admin.cleanupExpired').trigger('click')
    await flushPromises()
    expect(api.cleanup).toHaveBeenCalledWith({ task_id: 9, all: false })
    await findButton(wrapper, 'modelEvaluations.admin.cleanup').trigger('click')
    await wrapper.find('[data-testid="dialog"] input[type="checkbox"]').setValue(true)
    await findButton(wrapper, 'modelEvaluations.admin.cleanupAll').trigger('click')
    await flushPromises()
    expect(api.cleanup).toHaveBeenLastCalledWith({ task_id: undefined, all: true })
    await findButton(wrapper, 'modelEvaluations.admin.cleanup').trigger('click')
    expect(wrapper.find<HTMLInputElement>('[data-testid="dialog"] input[type="checkbox"]').element.checked).toBe(false)
    wrapper.unmount()
  })
})
