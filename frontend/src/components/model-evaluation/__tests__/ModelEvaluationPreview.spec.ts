import { mount, type VueWrapper } from '@vue/test-utils'
import { nextTick } from 'vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import ModelEvaluationPreview from '../ModelEvaluationPreview.vue'

let intersectionCallback: IntersectionObserverCallback
let resizeCallback: ResizeObserverCallback
let wrapper: VueWrapper | undefined
const disconnectIntersection = vi.fn()
const disconnectResize = vi.fn()
const html = '<html><body><svg id="pelican" viewBox="0 0 960 600"></svg><script>requestAnimationFrame(() => {})</script></body></html>'

async function visible(value = true) {
  intersectionCallback([{ target: wrapper!.element, isIntersecting: value } as IntersectionObserverEntry], {} as IntersectionObserver)
  await nextTick()
}

beforeEach(() => {
  vi.clearAllMocks()
  vi.spyOn(document, 'hidden', 'get').mockReturnValue(false)
  vi.stubGlobal('IntersectionObserver', class {
    constructor(callback: IntersectionObserverCallback) { intersectionCallback = callback }
    observe() {}
    disconnect = disconnectIntersection
  })
  vi.stubGlobal('ResizeObserver', class {
    constructor(callback: ResizeObserverCallback) { resizeCallback = callback }
    observe() {}
    disconnect = disconnectResize
  })
})

afterEach(() => {
  wrapper?.unmount()
  wrapper = undefined
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})

describe('ModelEvaluationPreview', () => {
  it('renders only an opaque-origin sandbox without permissions or parent markup', async () => {
    wrapper = mount(ModelEvaluationPreview, { props: { html, title: 'Configured model' } })
    expect(wrapper.find('iframe').exists()).toBe(false)
    await visible()
    const frame = wrapper.get('iframe')
    expect(frame.attributes('sandbox')).toBe('allow-scripts')
    expect(frame.attributes('referrerpolicy')).toBe('no-referrer')
    expect(frame.attributes('credentialless')).toBeDefined()
    expect(frame.attributes('tabindex')).toBe('-1')
    expect(frame.attributes('title')).toBe('Configured model')
    expect(frame.attributes('src')).toBeUndefined()
    expect(frame.attributes('srcdoc')).toContain('Content-Security-Policy')
    expect(frame.attributes('srcdoc')).toContain('requestAnimationFrame')
    expect(wrapper.find('svg').exists()).toBe(false)
    expect(wrapper.find('script').exists()).toBe(false)
  })

  it('destroys and recreates the iframe when active changes', async () => {
    wrapper = mount(ModelEvaluationPreview, { props: { html, active: false } })
    await visible()
    expect(wrapper.find('iframe').exists()).toBe(false)
    await wrapper.setProps({ active: true })
    const previous = wrapper.get('iframe').element
    await wrapper.setProps({ active: false })
    expect(wrapper.find('iframe').exists()).toBe(false)
    await wrapper.setProps({ active: true })
    expect(wrapper.get('iframe').element).not.toBe(previous)
  })

  it('removes animation frames when outside the viewport or the document is hidden', async () => {
    wrapper = mount(ModelEvaluationPreview, { props: { html } })
    await visible()
    expect(wrapper.find('iframe').exists()).toBe(true)
    await visible(false)
    expect(wrapper.find('iframe').exists()).toBe(false)
    await visible()
    vi.spyOn(document, 'hidden', 'get').mockReturnValue(true)
    document.dispatchEvent(new Event('visibilitychange'))
    await nextTick()
    expect(wrapper.find('iframe').exists()).toBe(false)
    vi.spyOn(document, 'hidden', 'get').mockReturnValue(false)
    document.dispatchEvent(new Event('visibilitychange'))
    await nextTick()
    expect(wrapper.find('iframe').exists()).toBe(true)
  })

  it('uniformly scales a stable viewport and updates document content without inserting it in the parent', async () => {
    wrapper = mount(ModelEvaluationPreview, { props: { html } })
    await visible()
    resizeCallback([{ target: wrapper.element, contentRect: { width: 320, height: 208 } } as ResizeObserverEntry], {} as ResizeObserver)
    await nextTick()
    expect(wrapper.get('iframe').attributes('style')).toContain('scale(0.3333333333333333)')
    resizeCallback([{ target: wrapper.element, contentRect: { width: 900, height: 300 } } as ResizeObserverEntry], {} as ResizeObserver)
    await nextTick()
    expect(wrapper.get('iframe').attributes('style')).toContain('scale(0.5)')
    await wrapper.setProps({ html: '<svg id="new-result"/>' })
    expect(wrapper.get('iframe').attributes('srcdoc')).toContain('new-result')
    expect(wrapper.find('#new-result').exists()).toBe(false)
  })

  it('does not create blank frames and disconnects all lifecycle observers on unmount', async () => {
    const removeListener = vi.spyOn(document, 'removeEventListener')
    wrapper = mount(ModelEvaluationPreview, { props: { html: '  ' }, attachTo: document.body })
    await visible()
    expect(wrapper.find('iframe').exists()).toBe(false)
    await wrapper.setProps({ html })
    const frame = wrapper.get('iframe').element
    expect(frame.isConnected).toBe(true)
    wrapper.unmount()
    wrapper = undefined
    expect(frame.isConnected).toBe(false)
    expect(disconnectIntersection).toHaveBeenCalledOnce()
    expect(disconnectResize).toHaveBeenCalledOnce()
    expect(removeListener).toHaveBeenCalledWith('visibilitychange', expect.any(Function))
  })

  it('still renders when browser observers are unavailable', async () => {
    vi.stubGlobal('IntersectionObserver', undefined)
    vi.stubGlobal('ResizeObserver', undefined)
    const removeListener = vi.spyOn(window, 'removeEventListener')
    wrapper = mount(ModelEvaluationPreview, { props: { html } })
    expect(wrapper.find('iframe').exists()).toBe(true)
    wrapper.unmount()
    wrapper = undefined
    expect(removeListener).toHaveBeenCalledWith('resize', expect.any(Function))
  })
})
