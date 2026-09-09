import { mount } from '@vue/test-utils'
import { defineComponent, h, ref } from 'vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const storageKey = 'subnexus_user_surface_mode'
let frames: Map<number, FrameRequestCallback>
let nextId: number
let now: number
let cleanup: (() => void) | undefined

function tick(duration: number, interval = 1000 / 60) {
  const end = now + duration
  while (now < end) {
    now += interval
    const callbacks = [...frames.values()]
    frames.clear()
    callbacks.forEach(callback => callback(now))
  }
}

beforeEach(() => {
  vi.resetModules()
  localStorage.clear()
  frames = new Map()
  nextId = 0
  now = 1
  vi.stubGlobal('requestAnimationFrame', vi.fn((callback: FrameRequestCallback) => {
    frames.set(++nextId, callback)
    return nextId
  }))
  vi.stubGlobal('cancelAnimationFrame', vi.fn((id: number) => frames.delete(id)))
  vi.spyOn(window, 'matchMedia').mockImplementation(query => ({ matches: false, media: query } as MediaQueryList))
  vi.spyOn(navigator, 'hardwareConcurrency', 'get').mockReturnValue(8)
  vi.spyOn(document, 'visibilityState', 'get').mockReturnValue('visible')
})

afterEach(() => {
  cleanup?.()
  cleanup = undefined
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})

async function mountMonitor(active = ref(true)) {
  const { useUserSurfacePerformance, useUserSurfacePerformanceMonitor } = await import('../useUserSurfacePerformance')
  const surface = useUserSurfacePerformance()
  const wrapper = mount(defineComponent({ setup() {
    useUserSurfacePerformanceMonitor(active)
    return () => h('div', surface.mode.value)
  } }))
  cleanup = () => wrapper.unmount()
  return { surface, wrapper, active }
}

describe('user surface performance preferences', () => {
  it('shares a persistent manual choice and restores automatic device selection', async () => {
    const { useUserSurfacePerformance } = await import('../useUserSurfacePerformance')
    const first = useUserSurfacePerformance()
    first.setMode('compat')
    const second = useUserSurfacePerformance()
    expect(second.mode.value).toBe('compat')
    expect(second.preference.value).toBe('compat')
    expect(localStorage.getItem(storageKey)).toBe('compat')
    first.setMode('auto')
    expect(second.mode.value).toBe('standard')
    expect(second.preference.value).toBe('auto')
    expect(localStorage.getItem(storageKey)).toBeNull()
  })

  it('keeps the application usable when storage reads and writes throw', async () => {
    vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => { throw new Error('blocked') })
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => { throw new Error('quota') })
    vi.spyOn(Storage.prototype, 'removeItem').mockImplementation(() => { throw new Error('blocked') })
    const { useUserSurfacePerformance } = await import('../useUserSurfacePerformance')
    const surface = useUserSurfacePerformance()
    expect(surface.mode.value).toBe('standard')
    expect(() => surface.setMode('compat')).not.toThrow()
    expect(surface.mode.value).toBe('compat')
    expect(() => surface.setMode('auto')).not.toThrow()
    expect(surface.preference.value).toBe('auto')
  })

  it.each(['mobile', 'motion', 'cpu'])('selects a static lightweight default for %s devices', async reason => {
    if (reason === 'cpu') vi.spyOn(navigator, 'hardwareConcurrency', 'get').mockReturnValue(4)
    else vi.spyOn(window, 'matchMedia').mockImplementation(query => ({
      matches: query.includes(reason === 'mobile' ? 'max-width' : 'reduced-motion'), media: query,
    } as MediaQueryList))
    const { useUserSurfacePerformance } = await import('../useUserSurfacePerformance')
    expect(useUserSurfacePerformance().mode.value).toBe('balanced')
  })

  it('honors stored manual choices even on low core devices and rejects invalid storage', async () => {
    vi.spyOn(navigator, 'hardwareConcurrency', 'get').mockReturnValue(2)
    localStorage.setItem(storageKey, 'standard')
    let api = await import('../useUserSurfacePerformance')
    expect(api.useUserSurfacePerformance().mode.value).toBe('standard')
    localStorage.setItem(storageKey, 'not-a-mode')
    vi.resetModules()
    api = await import('../useUserSurfacePerformance')
    expect(api.useUserSurfacePerformance().mode.value).toBe('balanced')
    expect(api.useUserSurfacePerformance().hasManualPreference.value).toBe(false)
  })
})

describe('automatic performance monitoring', () => {
  it('requires sustained slow windows and never saves an automatic downgrade as manual', async () => {
    const { surface } = await mountMonitor()
    tick(6000, 50)
    expect(surface.mode.value).toBe('standard')
    tick(2500, 50)
    expect(surface.mode.value).toBe('balanced')
    expect(surface.preference.value).toBe('auto')
    expect(localStorage.getItem(storageKey)).toBeNull()
    expect(frames.size).toBe(0)
  })

  it('ignores initial load, temporary stalls, and background-tab time', async () => {
    const { surface } = await mountMonitor()
    tick(6500, 50)
    vi.spyOn(document, 'visibilityState', 'get').mockReturnValue('hidden')
    document.dispatchEvent(new Event('visibilitychange'))
    expect(frames.size).toBe(0)
    tick(120000, 1000)
    vi.spyOn(document, 'visibilityState', 'get').mockReturnValue('visible')
    document.dispatchEvent(new Event('visibilitychange'))
    tick(1500, 100)
    tick(10000)
    expect(surface.mode.value).toBe('standard')
  })

  it('manual selection and leaving the user surface stop all monitoring', async () => {
    const { surface, active, wrapper } = await mountMonitor()
    surface.setMode('standard')
    expect(frames.size).toBe(0)
    tick(15000, 50)
    expect(surface.mode.value).toBe('standard')
    surface.setMode('auto')
    expect(frames.size).toBe(1)
    active.value = false
    expect(frames.size).toBe(0)
    active.value = true
    expect(frames.size).toBe(1)
    wrapper.unmount()
    expect(frames.size).toBe(0)
    cleanup = undefined
  })
})
