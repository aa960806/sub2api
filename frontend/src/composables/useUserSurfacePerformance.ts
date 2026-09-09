import { computed, onBeforeUnmount, onMounted, readonly, ref, watch, type Ref } from 'vue'

export type UserSurfaceMode = 'standard' | 'balanced' | 'compat'
export type UserSurfacePreference = UserSurfaceMode | 'auto'

const STORAGE_KEY = 'subnexus_user_surface_mode'
const mode = ref<UserSurfaceMode>('standard')
const hasManualPreference = ref(false)
let initialized = false

function isMode(value: unknown): value is UserSurfaceMode {
  return value === 'standard' || value === 'balanced' || value === 'compat'
}

function detectDefaultMode(): UserSurfaceMode {
  if (typeof window === 'undefined') return 'standard'

  const reducedMotion = window.matchMedia?.('(prefers-reduced-motion: reduce)').matches
  const mobile = window.matchMedia?.('(max-width: 767px)').matches
  const deviceMemory = typeof navigator !== 'undefined'
    ? (navigator as Navigator & { deviceMemory?: number }).deviceMemory
    : undefined
  const lowMemory = typeof deviceMemory === 'number' && deviceMemory > 0 && deviceMemory <= 4
  const lowCpu = typeof navigator !== 'undefined' && typeof navigator.hardwareConcurrency === 'number'
    && navigator.hardwareConcurrency > 0 && navigator.hardwareConcurrency <= 4

  return reducedMotion || mobile || lowMemory || lowCpu ? 'balanced' : 'standard'
}

function initialize() {
  if (initialized || typeof window === 'undefined') return
  initialized = true
  let stored: string | null = null
  try {
    stored = window.localStorage.getItem(STORAGE_KEY)
  } catch {
    // Blocked storage must never prevent the application from opening.
  }
  hasManualPreference.value = isMode(stored)
  mode.value = isMode(stored) ? stored : detectDefaultMode()
}

function setMode(preference: UserSurfacePreference) {
  if (preference !== 'auto' && !isMode(preference)) return
  hasManualPreference.value = preference !== 'auto'
  mode.value = preference === 'auto' ? detectDefaultMode() : preference
  try {
    if (preference === 'auto') window.localStorage.removeItem(STORAGE_KEY)
    else window.localStorage.setItem(STORAGE_KEY, preference)
  } catch {
    // Keep a usable in-memory preference in private/blocked storage contexts.
  }
}

export function useUserSurfacePerformance() {
  initialize()
  return {
    mode: readonly(mode),
    hasManualPreference: readonly(hasManualPreference),
    preference: computed<UserSurfacePreference>(() => hasManualPreference.value ? mode.value : 'auto'),
    setMode,
    isGlassEnabled: computed(() => mode.value !== 'compat'),
    animationsEnabled: computed(() => mode.value === 'standard'),
    quality: computed<'standard' | 'balanced'>(() => mode.value === 'standard' ? 'standard' : 'balanced'),
  }
}

/** Measure only visible, automatic standard mode. An isolated stall is not a downgrade. */
export function useUserSurfacePerformanceMonitor(active: Readonly<Ref<boolean>>) {
  const mounted = ref(false)
  const visible = ref(false)
  let frameId = 0
  const stop = () => {
    if (frameId) window.cancelAnimationFrame(frameId)
    frameId = 0
  }
  const syncVisibility = () => { visible.value = document.visibilityState === 'visible' }

  watch([active, mounted, visible, mode, hasManualPreference], () => {
    stop()
    if (!mounted.value || !active.value || !visible.value || hasManualPreference.value
      || mode.value !== 'standard' || typeof window.requestAnimationFrame !== 'function') return

    let previous = 0
    let warmupUntil = 0
    let windowStarted = 0
    let frames = 0
    let slowWindows = 0
    const probe = (now: number) => {
      frameId = 0
      if (!previous || now - previous > 1000) {
        // Resume/suspended-device gaps and initial loading get a fresh warmup.
        warmupUntil = now + 2000
        windowStarted = warmupUntil
        frames = 0
        slowWindows = 0
      }
      previous = now
      if (now >= warmupUntil) {
        frames += 1
        const elapsed = now - windowStarted
        if (elapsed >= 2000) {
          slowWindows = frames * 1000 / elapsed < 40 ? slowWindows + 1 : 0
          frames = 0
          windowStarted = now
          if (slowWindows >= 3) {
            // Automatic decisions are session-only and never become a manual preference.
            if (!hasManualPreference.value && mode.value === 'standard') mode.value = 'balanced'
            return
          }
        }
      }
      frameId = window.requestAnimationFrame(probe)
    }
    frameId = window.requestAnimationFrame(probe)
  }, { flush: 'sync' })

  onMounted(() => {
    syncVisibility()
    mounted.value = true
    document.addEventListener('visibilitychange', syncVisibility)
  })
  onBeforeUnmount(() => {
    mounted.value = false
    stop()
    document.removeEventListener('visibilitychange', syncVisibility)
  })
}
