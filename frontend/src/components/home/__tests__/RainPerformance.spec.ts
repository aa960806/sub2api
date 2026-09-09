import { mount, type VueWrapper } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import GlassDropletsCanvas from '../GlassDropletsCanvas.vue'
import RainStreaksCanvas from '../RainStreaksCanvas.vue'
import RainyBackground from '../RainyBackground.vue'

let wrapper: VueWrapper | undefined
let now: number
let nextId: number
let frames: Map<number, FrameRequestCallback>
let paints: ReturnType<typeof vi.fn>
let droplets: ReturnType<typeof vi.fn>

function tick(count = 1, interval = 1000 / 60) {
  for (let i = 0; i < count; i++) {
    now += interval
    const callbacks = [...frames.values()]
    frames.clear()
    callbacks.forEach(callback => callback(now))
  }
}

beforeEach(() => {
  now = 0
  nextId = 0
  frames = new Map()
  paints = vi.fn()
  droplets = vi.fn(() => ({ addColorStop: vi.fn() }))
  vi.spyOn(navigator, 'userAgent', 'get').mockReturnValue('Chrome')
  vi.spyOn(performance, 'now').mockImplementation(() => now)
  vi.stubGlobal('requestAnimationFrame', vi.fn((callback: FrameRequestCallback) => {
    frames.set(++nextId, callback)
    return nextId
  }))
  vi.stubGlobal('cancelAnimationFrame', vi.fn((id: number) => frames.delete(id)))
  const noop = () => undefined
  const context = new Proxy({
    clearRect: paints,
    createRadialGradient: droplets,
    createLinearGradient: () => ({ addColorStop: vi.fn() }),
  }, { get: (target, key) => key in target ? Reflect.get(target, key) : noop })
  vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockReturnValue(context as unknown as CanvasRenderingContext2D)
})

afterEach(() => {
  wrapper?.unmount()
  wrapper = undefined
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})

describe('rain rendering lifecycle', () => {
  it('renders only sparse static droplets in balanced mode without a frame or click loop', async () => {
    wrapper = mount(RainyBackground, { props: { quality: 'balanced', animated: true, mouseX: 100, mouseY: 200 } })
    expect(wrapper.findComponent(RainStreaksCanvas).exists()).toBe(false)
    expect(wrapper.findComponent(GlassDropletsCanvas).props('animated')).toBe(false)
    expect(droplets.mock.calls.length).toBeGreaterThan(0)
    expect(droplets.mock.calls.length).toBeLessThanOrEqual(24)
    expect(frames.size).toBe(0)
    const transform = wrapper.get('.absolute.-inset-8').attributes('style')
    await wrapper.setProps({ mouseX: 700, mouseY: 600 })
    expect(wrapper.get('.absolute.-inset-8').attributes('style')).toBe(transform)
    const count = paints.mock.calls.length
    window.dispatchEvent(new MouseEvent('click', { clientX: 100, clientY: 100 }))
    tick(120)
    expect(paints).toHaveBeenCalledTimes(count)
    window.dispatchEvent(new Event('resize'))
    expect(paints).toHaveBeenCalledTimes(count + 1)
    expect(frames.size).toBe(0)
  })

  it.each([GlassDropletsCanvas, RainStreaksCanvas])('paints smoothly at 60 Hz and releases loops when paused', async component => {
    wrapper = mount(component, { props: { enabled: true } })
    tick(60)
    expect(paints.mock.calls.length).toBeGreaterThanOrEqual(57)
    await wrapper.setProps({ animated: false })
    expect(frames.size).toBe(0)
    const pausedPaints = paints.mock.calls.length
    tick(60)
    expect(paints).toHaveBeenCalledTimes(pausedPaints)
    await wrapper.setProps({ animated: true })
    expect(frames.size).toBe(1)
    wrapper.unmount()
    wrapper = undefined
    expect(frames.size).toBe(0)
  })

  it('bounds droplet work after repeated page clicks instead of retaining unlimited particles', () => {
    wrapper = mount(GlassDropletsCanvas, { props: { enabled: true } })
    for (let round = 0; round < 3; round++) {
      for (let i = 0; i < 200; i++) window.dispatchEvent(new MouseEvent('click', { clientX: i, clientY: i }))
      droplets.mockClear()
      tick()
      expect(droplets.mock.calls.length).toBeGreaterThan(0)
      expect(droplets.mock.calls.length).toBeLessThanOrEqual(108)
    }
  })

  it('removes the teleported canvas and its resize handler on unmount', () => {
    wrapper = mount(GlassDropletsCanvas, { props: { enabled: true, animated: false, quality: 'balanced' } })
    expect(document.body.querySelector('canvas')).not.toBeNull()
    wrapper.unmount()
    wrapper = undefined
    paints.mockClear()
    window.dispatchEvent(new Event('resize'))
    expect(document.body.querySelector('canvas')).toBeNull()
    expect(paints).not.toHaveBeenCalled()
    expect(frames.size).toBe(0)
  })
})
