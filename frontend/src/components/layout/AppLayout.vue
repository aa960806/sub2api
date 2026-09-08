<template>
  <div
    class="app-layout-root relative min-h-screen bg-gray-50 dark:bg-dark-950"
    :class="{
      'user-glass-surface': isUserSurface,
      'subnexus-legacy-surface': legacySurface,
    }"
  >
    <!-- Keep the homepage rain treatment behind authenticated user pages only. -->
    <RainyBackground
      v-if="isUserSurface"
      :enabled="true"
      :animated="surfaceAnimationsEnabled"
      :image-index="2"
      :mouse-x="surfaceMouseX"
      :mouse-y="surfaceMouseY"
      :droplets-z-index="9"
      theme="deep-night"
    />

    <div v-if="isUserSurface" class="user-surface-wash" aria-hidden="true"></div>

    <!-- Background Decoration -->
    <div v-if="!isUserSurface" class="pointer-events-none fixed inset-0 bg-mesh-gradient"></div>

    <!-- Sidebar -->
    <AppSidebar />

    <!-- Main Content Area -->
    <div
      class="relative min-h-screen transition-all duration-300"
      :class="[
        sidebarCollapsed ? 'lg:ml-[72px]' : 'lg:ml-64',
        { 'user-surface-content': isUserSurface },
      ]"
    >
      <!-- Header -->
      <AppHeader :show-page-description="!legacySurface" />

      <!-- Main Content -->
      <main class="p-4 md:p-6 lg:p-8">
        <slot />
      </main>
    </div>
  </div>
</template>

<script setup lang="ts">
import '@/styles/onboarding.css'
import '@/styles/subnexus-legacy-surface.css'
import '@/styles/user-glass-surface.css'
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { useAppStore } from '@/stores'
import { useAuthStore } from '@/stores/auth'
import { useOnboardingTour } from '@/composables/useOnboardingTour'
import { useOnboardingStore } from '@/stores/onboarding'
import AppSidebar from './AppSidebar.vue'
import AppHeader from './AppHeader.vue'
import RainyBackground from '@/components/home/RainyBackground.vue'

withDefaults(defineProps<{
  legacySurface?: boolean
}>(), {
  legacySurface: false,
})

const appStore = useAppStore()
const authStore = useAuthStore()
const route = useRoute()
const sidebarCollapsed = computed(() => appStore.sidebarCollapsed)
const isAdmin = computed(() => authStore.user?.role === 'admin')
const isUserSurface = computed(() => (
  route.meta.requiresAuth === true && route.meta.requiresAdmin !== true
))

const surfaceMouseX = ref(typeof window === 'undefined' ? 0 : window.innerWidth / 2)
const surfaceMouseY = ref(typeof window === 'undefined' ? 0 : window.innerHeight / 2)
const surfaceAnimationsEnabled = ref(true)
let reducedMotionQuery: MediaQueryList | null = null
let surfacePointerTrackingAttached = false

function syncSurfaceAnimationState() {
  const shouldAnimate = !reducedMotionQuery?.matches && document.visibilityState === 'visible'
  surfaceAnimationsEnabled.value = shouldAnimate

  if (shouldAnimate && !surfacePointerTrackingAttached) {
    window.addEventListener('pointermove', handleSurfacePointerMove, { passive: true })
    surfacePointerTrackingAttached = true
  } else if (!shouldAnimate && surfacePointerTrackingAttached) {
    window.removeEventListener('pointermove', handleSurfacePointerMove)
    surfacePointerTrackingAttached = false
  }
}

function handleSurfacePointerMove(event: PointerEvent) {
  surfaceMouseX.value = event.clientX
  surfaceMouseY.value = event.clientY
}

function handleReducedMotionChange() {
  syncSurfaceAnimationState()
}

const { replayTour } = useOnboardingTour({
  storageKey: isAdmin.value ? 'admin_guide' : 'user_guide',
  autoStart: true
})

const onboardingStore = useOnboardingStore()

onMounted(() => {
  onboardingStore.setReplayCallback(replayTour)

  if (isUserSurface.value) {
    reducedMotionQuery = window.matchMedia('(prefers-reduced-motion: reduce)')
    syncSurfaceAnimationState()
    document.addEventListener('visibilitychange', syncSurfaceAnimationState)
    reducedMotionQuery.addEventListener?.('change', handleReducedMotionChange)
  }
})

onBeforeUnmount(() => {
  if (surfacePointerTrackingAttached) {
    window.removeEventListener('pointermove', handleSurfacePointerMove)
    surfacePointerTrackingAttached = false
  }
  document.removeEventListener('visibilitychange', syncSurfaceAnimationState)
  reducedMotionQuery?.removeEventListener?.('change', handleReducedMotionChange)
  reducedMotionQuery = null
})

defineExpose({ replayTour })
</script>

<style scoped>
html.dark .app-layout-root.subnexus-legacy-surface:not(.user-glass-surface) {
  background-color: #0b1220;
}
</style>
