import { defineComponent, nextTick } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import ModelEvaluationGallery from '../ModelEvaluationGallery.vue'
import type { ModelEvaluationResult } from '@/api/modelEvaluations'

const api = vi.hoisted(() => ({ results: vi.fn(), result: vi.fn(), deleteResult: vi.fn() }))
vi.mock('@/api/modelEvaluations', () => ({ modelEvaluationsAPI: api, adminModelEvaluationsAPI: api }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showSuccess: vi.fn(), showError: vi.fn() }) }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key, locale: { value: 'en' } }) }))
const Preview = defineComponent({ name: 'ModelEvaluationPreview', props: ['html'], template: '<div data-testid="animation">{{ html }}</div>' })
const Dialog = defineComponent({ props: ['show'], emits: ['close'], template: '<div v-if="show" data-testid="dialog"><button data-testid="close" @click="$emit(\'close\')" /><slot /><slot name="footer" /></div>' })
const record = (id: number): ModelEvaluationResult => ({ id, task_id: 1, task_name: 'Task', group_id: 1, group_name: 'Group', model: 'model-a', status: 'success', duration_ms: 2000, created_at: '2026-09-11T00:00:00Z' })
function render() {
  return mount(ModelEvaluationGallery, { props: { groups: [{ id: 1, name: 'A' }, { id: 2, name: 'B' }] }, global: { stubs: { ModelEvaluationPreview: Preview, BaseDialog: Dialog, Teleport: true } } })
}
describe('model evaluation gallery', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    Object.defineProperty(document, 'hidden', { configurable: true, value: false })
    vi.stubGlobal('IntersectionObserver', undefined)
    api.results.mockResolvedValue({ items: [1, 2, 3, 4].map(record), page: 1, page_size: 12, total: 4 })
    api.result.mockImplementation(async (id: number) => ({ ...record(id), html: `<html>${id}</html>` }))
  })
  afterEach(() => { vi.unstubAllGlobals(); vi.useRealTimers() })

  it('fetches HTML only for two active previews and suspends them while enlarged', async () => {
    const wrapper = render()
    await flushPromises()
    expect(api.result).toHaveBeenCalledTimes(2)
    expect(wrapper.findAll('[data-testid="animation"]')).toHaveLength(2)
    await wrapper.findAll('article > button')[2].trigger('click')
    await flushPromises()
    expect(api.result).toHaveBeenCalledTimes(3)
    expect(wrapper.findAll('[data-testid="animation"]')).toHaveLength(1)
    expect(wrapper.find('[data-testid="dialog"]').text()).toContain('<html>3</html>')
    await wrapper.find('[data-testid="close"]').trigger('click')
    await flushPromises()
    expect(wrapper.findAll('[data-testid="animation"]')).toHaveLength(2)
    wrapper.unmount()
  })

  it('unloads running animations while the document is hidden', async () => {
    const wrapper = render()
    await flushPromises()
    Object.defineProperty(document, 'hidden', { configurable: true, value: true })
    document.dispatchEvent(new Event('visibilitychange'))
    await nextTick()
    expect(wrapper.findAll('[data-testid="animation"]')).toHaveLength(0)
    Object.defineProperty(document, 'hidden', { configurable: true, value: false })
    document.dispatchEvent(new Event('visibilitychange'))
    await nextTick()
    expect(wrapper.findAll('[data-testid="animation"]')).toHaveLength(2)
    wrapper.unmount()
  })

  it('enlarges on hover without keeping thumbnail animations running', async () => {
    vi.stubGlobal('matchMedia', vi.fn(() => ({ matches: true })))
    const wrapper = render()
    await flushPromises()
    vi.useFakeTimers()
    await wrapper.findAll('article > button')[0].trigger('mouseenter')
    await vi.advanceTimersByTimeAsync(250)
    await nextTick()
    expect(wrapper.find('[data-testid="hover-preview"]').exists()).toBe(true)
    expect(wrapper.findAll('[data-testid="animation"]')).toHaveLength(1)
    await wrapper.findAll('article > button')[0].trigger('mouseleave')
    await nextTick()
    expect(wrapper.find('[data-testid="hover-preview"]').exists()).toBe(false)
    expect(wrapper.findAll('[data-testid="animation"]')).toHaveLength(2)
    wrapper.unmount()
  })

  it('aborts old group requests and ignores late responses', async () => {
    let resolveOld!: (value: unknown) => void
    api.results.mockImplementationOnce(() => new Promise(resolve => { resolveOld = resolve }))
    const wrapper = render()
    await flushPromises()
    const oldSignal = api.results.mock.calls[0][1] as AbortSignal
    await wrapper.find('[data-testid="group-filter"]').setValue(2)
    await flushPromises()
    expect(oldSignal.aborted).toBe(true)
    expect(api.results.mock.calls[1][0]).toMatchObject({ group_id: 2, page: 1, page_size: 12 })
    resolveOld({ items: [{ ...record(99), group_name: 'STALE' }], page: 1, page_size: 12, total: 1 })
    await flushPromises()
    expect(wrapper.text()).not.toContain('STALE')
    expect(wrapper.findAll('article')).toHaveLength(4)
    wrapper.unmount()
  })

  it('hides failure details from users even when returned by the API', async () => {
    api.results.mockResolvedValue({ items: [{ ...record(1), status: 'error', error_message: '<script>alert(1)</script>' }], total: 1 })
    const wrapper = render()
    await flushPromises()
    expect(api.result).not.toHaveBeenCalled()
    expect(wrapper.find('script').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('<script>alert(1)</script>')
    expect(wrapper.text()).toContain('modelEvaluations.generationFailed')
    wrapper.unmount()
  })

  it('keeps escaped failure details and private test markers visible to admins only', async () => {
    api.results.mockResolvedValue({ items: [{ ...record(1), status: 'error', error_message: '<script>failure</script>', is_test: true }], total: 1 })
    const wrapper = render()
    await wrapper.setProps({ admin: true })
    await flushPromises()
    expect(wrapper.text()).toContain('<script>failure</script>')
    expect(wrapper.find('script').exists()).toBe(false)
    expect(wrapper.text()).toContain('modelEvaluations.admin.privateTest')
    wrapper.unmount()
  })
})
