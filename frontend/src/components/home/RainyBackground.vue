<script setup lang="ts">
import { computed, nextTick, onMounted, ref, watch } from 'vue'
import GlassDropletsCanvas from './GlassDropletsCanvas.vue'
import RainStreaksCanvas from './RainStreaksCanvas.vue'

type GlassTheme = 'cool-slate' | 'deep-night' | 'cyan-mist'
type RainQuality = 'standard' | 'balanced'

interface RainyBackgroundProps {
  /** Enables the rain and condensation animation layers. */
  enabled?: boolean
  /** Keeps a static rain frame visible while motion is reduced or paused. */
  animated?: boolean
  quality?: RainQuality
  /** Selects the image from the target page's rotating city set. */
  imageIndex?: number
  mouseX?: number
  mouseY?: number
  /** Controls the teleported droplet layer relative to host content. */
  dropletsZIndex?: number
  theme?: GlassTheme
  /** Allows the host page to provide a different, configured image set. */
  images?: string[]
}

const props = withDefaults(defineProps<RainyBackgroundProps>(), {
  enabled: true,
  animated: true,
  quality: 'standard' as RainQuality,
  imageIndex: 0,
  mouseX: 0,
  mouseY: 0,
  dropletsZIndex: 30,
  theme: 'deep-night' as GlassTheme,
  images: () => [
    // The embedded Go frontend reserves /images/* for the API image
    // endpoints, so public background assets must live at the root path.
    '/rain-city-1.jpg',
    '/rain-city-2.jpg',
    '/rain-city-3.jpg',
  ],
})

const imageLoaded = ref(false)
const imageElement = ref<HTMLImageElement | null>(null)

const currentImage = computed(() => {
  if (props.images.length === 0) return ''
  const index = Math.trunc(props.imageIndex)
  const normalizedIndex = ((index % props.images.length) + props.images.length) % props.images.length
  return props.images[normalizedIndex] || ''
})

function syncImageLoadState() {
  const element = imageElement.value
  if (element?.complete && element.naturalWidth > 0) {
    imageLoaded.value = true
  }
}

function handleImageLoad() {
  imageLoaded.value = true
}

watch(currentImage, async () => {
  imageLoaded.value = false
  await nextTick()
  syncImageLoadState()
}, { immediate: true })

onMounted(() => {
  syncImageLoadState()
})

const parallaxStyle = computed(() => {
  if (!props.enabled || !props.animated || props.quality === 'balanced' || typeof window === 'undefined') {
    return { transform: 'translate3d(0, 0, 0) scale(1.04)' }
  }

  const width = window.innerWidth || 1
  const height = window.innerHeight || 1
  const offsetX = ((props.mouseX / width) - 0.5) * -12
  const offsetY = ((props.mouseY / height) - 0.5) * -12

  return {
    transform: `translate3d(${offsetX}px, ${offsetY}px, 0) scale(1.04)`,
  }
})

const themeOverlays: Record<GlassTheme, string> = {
  'cool-slate': 'linear-gradient(to bottom, rgba(5, 12, 24, 0.28), rgba(7, 19, 36, 0.32), rgba(4, 8, 18, 0.48))',
  'deep-night': 'linear-gradient(to bottom, rgba(3, 7, 18, 0.30), rgba(6, 14, 29, 0.36), rgba(2, 5, 14, 0.52))',
  'cyan-mist': 'linear-gradient(to bottom, rgba(3, 15, 24, 0.25), rgba(5, 24, 36, 0.31), rgba(2, 10, 16, 0.46))',
}

const imageStyle = {
  filter: 'blur(2px) brightness(0.84) contrast(1.08) saturate(1.12)',
}
</script>

<template>
  <div
    class="rainy-background pointer-events-none fixed inset-0 z-0 select-none overflow-hidden bg-[#040812]"
    aria-hidden="true"
  >
    <div
      class="absolute -inset-8"
      :class="{ 'will-change-transform': props.enabled && props.animated && props.quality === 'standard' }"
      :style="{ ...parallaxStyle, transition: props.animated ? 'transform 500ms ease-out' : 'none' }"
    >
      <img
        v-if="currentImage"
        ref="imageElement"
        :src="currentImage"
        alt=""
        draggable="false"
        class="h-full w-full object-cover object-center transition-opacity duration-1000"
        :class="imageLoaded ? 'opacity-100' : 'opacity-0'"
        :style="imageStyle"
        @load="handleImageLoad"
      />
      <div v-if="!imageLoaded" class="absolute inset-0 flex items-center justify-center bg-[#050c18]">
        <div class="h-full w-full bg-[radial-gradient(ellipse_at_center,_var(--tw-gradient-stops))] from-slate-900 via-[#06101f] to-[#03060f]" />
      </div>
    </div>

    <div
      class="pointer-events-none absolute inset-0 transition-[background] duration-700"
      :style="{ backgroundImage: themeOverlays[props.theme] }"
    />
    <div
      class="pointer-events-none absolute inset-0 bg-[radial-gradient(circle_at_center,transparent_48%,rgba(3,7,16,0.34)_100%)]"
    />

    <RainStreaksCanvas v-if="props.quality === 'standard'" :enabled="props.enabled" :animated="props.animated" :quality="props.quality" :intensity="0.85" :wind-speed="2.8" />
    <GlassDropletsCanvas
      :enabled="props.enabled"
      :animated="props.animated && props.quality === 'standard'"
      :quality="props.quality"
      :mouse-x="props.mouseX"
      :mouse-y="props.mouseY"
      :z-index="props.dropletsZIndex"
    />
  </div>
</template>
