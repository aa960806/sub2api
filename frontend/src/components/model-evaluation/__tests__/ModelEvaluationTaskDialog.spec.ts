import { defineComponent } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import ModelEvaluationTaskDialog from '../ModelEvaluationTaskDialog.vue'
import type { ModelEvaluationTask } from '@/api/modelEvaluations'
const api = vi.hoisted(() => ({ create: vi.fn(), update: vi.fn() }))
vi.mock('@/api/modelEvaluations', () => ({ adminModelEvaluationsAPI: api, MODEL_EVALUATION_PROMPT: '生成html，内容是svg绘制鹈鹕骑自行车2D动画，不用进行测试。' }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
const Dialog = defineComponent({ props: ['show'], template: '<div v-if="show"><slot /><slot name="footer" /></div>' })
const task: ModelEvaluationTask = { id: 4, name: 'Task', group_id: 7, group_name: 'Group', endpoint: 'https://api.example.com/v1/messages', api_format: 'messages', model: 'claude-example', enabled: true, interval_seconds: 3600, retention_days: 7, max_records: 50, has_api_key: true, next_run_at: null, created_at: '', updated_at: '', published: false, test_status: 'untested', last_tested_at: null, test_error: '' }
describe('model evaluation task editor', () => {
  beforeEach(() => vi.clearAllMocks())
  it('preserves the saved key on edit and submits the exact model and API protocol', async () => {
    const wrapper = mount(ModelEvaluationTaskDialog, { props: { show: true, task, groups: [{ id: 7, name: 'Group' }] }, global: { stubs: { BaseDialog: Dialog } } })
    const key = wrapper.find<HTMLInputElement>('[name="api_key"]')
    expect(key.element.value).toBe('')
    expect(key.attributes('type')).toBe('password')
    await wrapper.find('[name="model"]').setValue('provider/model-version')
    await wrapper.find('form').trigger('submit')
    await flushPromises()
    expect(api.update).toHaveBeenCalledWith(4, expect.objectContaining({ api_key: undefined, model: 'provider/model-version', api_format: 'messages' }))
    expect(wrapper.emitted('saved')).toHaveLength(1)
    wrapper.unmount()
  })

  it('clears a newly entered key when the editor closes', async () => {
    const wrapper = mount(ModelEvaluationTaskDialog, { props: { show: true, task: null, groups: [] }, global: { stubs: { BaseDialog: Dialog } } })
    await wrapper.find('[name="api_key"]').setValue('test-key')
    await wrapper.setProps({ show: false })
    await wrapper.setProps({ show: true })
    expect(wrapper.find<HTMLInputElement>('[name="api_key"]').element.value).toBe('')
    expect(wrapper.find<HTMLInputElement>('[name="interval_seconds"]').element.value).toBe('3600')
    expect(wrapper.find<HTMLInputElement>('[name="max_records"]').element.value).toBe('50')
    wrapper.unmount()
  })

  it.each(['endpoint', 'api_format', 'group_id'])('requires a key when changing the saved %s binding', async field => {
    const wrapper = mount(ModelEvaluationTaskDialog, { props: { show: true, task, groups: [{ id: 7, name: 'Group' }, { id: 8, name: 'Other' }] }, global: { stubs: { BaseDialog: Dialog } } })
    const value = field === 'endpoint' ? 'https://new.example.com/v1/messages' : field === 'api_format' ? 'responses' : '8'
    await wrapper.find(`[name="${field}"]`).setValue(value)
    expect(wrapper.find('[name="api_key"]').attributes('required')).toBeDefined()
    await wrapper.find('form').trigger('submit')
    expect(api.update).not.toHaveBeenCalled()
    await wrapper.find('[name="api_key"]').setValue('new-test-key')
    await wrapper.find('form').trigger('submit')
    await flushPromises()
    expect(api.update).toHaveBeenCalledWith(4, expect.objectContaining({ api_key: 'new-test-key' }))
    wrapper.unmount()
  })

  it('rejects a bare website URL without rewriting it or saving', async () => {
    const wrapper = mount(ModelEvaluationTaskDialog, { props: { show: true, task, groups: [{ id: 7, name: 'Group' }] }, global: { stubs: { BaseDialog: Dialog } } })
    await wrapper.find('[name="endpoint"]').setValue('https://gateway.example.com/')
    await wrapper.find('[name="api_key"]').setValue('test-key')
    await wrapper.find('form').trigger('submit')
    expect(api.update).not.toHaveBeenCalled()
    expect(wrapper.find('#model-evaluation-endpoint-error').text()).toBe('modelEvaluations.admin.endpointPathRequired')
    expect(wrapper.find<HTMLInputElement>('[name="endpoint"]').element.value).toBe('https://gateway.example.com/')
    wrapper.unmount()
  })

  it('maps safe server codes without showing arbitrary error content', async () => {
    api.update.mockRejectedValueOnce({ code: 'MODEL_EVALUATION_ENDPOINT_PATH_REQUIRED', message: 'secret-token' })
    const wrapper = mount(ModelEvaluationTaskDialog, { props: { show: true, task, groups: [{ id: 7, name: 'Group' }] }, global: { stubs: { BaseDialog: Dialog } } })
    await wrapper.find('form').trigger('submit')
    await flushPromises()
    expect(wrapper.text()).toContain('modelEvaluations.admin.endpointPathRequired')
    expect(wrapper.text()).not.toContain('secret-token')
    wrapper.unmount()
  })
})
