<template>
  <div ref="container" class="model-evaluation-preview">
    <iframe
      v-if="shouldRender"
      class="model-evaluation-preview__frame"
      :style="{ transform: `translate(-50%, -50%) scale(${scale})` }"
      :srcdoc="sandboxDocument"
      :title="title"
      sandbox="allow-scripts"
      referrerpolicy="no-referrer"
      credentialless
      allow="camera 'none'; microphone 'none'; geolocation 'none'; clipboard-read 'none'; clipboard-write 'none'; fullscreen 'none'; payment 'none'; autoplay 'none'"
      tabindex="-1"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { createModelEvaluationDocument } from '@/utils/modelEvaluationSandbox'

const props = withDefaults(defineProps<{ html: string; active?: boolean; title?: string }>(), {
  active: true,
  title: 'HTML preview',
})

const container = ref<HTMLElement | null>(null)
const documentVisible = ref(!document.hidden)
const inViewport = ref(typeof IntersectionObserver === 'undefined')
const scale = ref(1)
let intersectionObserver: IntersectionObserver | undefined
let resizeObserver: ResizeObserver | undefined
let disposed = false

const shouldRender = computed(() => props.active && documentVisible.value && inViewport.value && !!props.html.trim())
const sandboxDocument = computed(() => shouldRender.value ? createModelEvaluationDocument(props.html) : '')

function updateScale(width: number, height: number) {
  if (width > 0 && height > 0) scale.value = Math.min(width / 960, height / 600)
}

function measureContainer() {
  const bounds = container.value?.getBoundingClientRect()
  if (bounds) updateScale(bounds.width, bounds.height)
}

function onVisibilityChange() {
  documentVisible.value = !document.hidden
}

onMounted(() => {
  document.addEventListener('visibilitychange', onVisibilityChange)
  measureContainer()
  if (typeof ResizeObserver !== 'undefined') {
    resizeObserver = new ResizeObserver(entries => {
      if (disposed) return
      const entry = entries.find(item => item.target === container.value)
      if (entry) updateScale(entry.contentRect.width, entry.contentRect.height)
    })
    if (container.value) resizeObserver.observe(container.value)
  } else {
    window.addEventListener('resize', measureContainer)
  }
  if (typeof IntersectionObserver !== 'undefined') {
    intersectionObserver = new IntersectionObserver(entries => {
      if (disposed) return
      const entry = entries.find(item => item.target === container.value)
      if (entry) inViewport.value = entry.isIntersecting
    })
    if (container.value) intersectionObserver.observe(container.value)
  }
})

onBeforeUnmount(() => {
  disposed = true
  intersectionObserver?.disconnect()
  resizeObserver?.disconnect()
  document.removeEventListener('visibilitychange', onVisibilityChange)
  window.removeEventListener('resize', measureContainer)
})
</script>

<style scoped>
.model-evaluation-preview {
  position: relative;
  width: 100%;
  height: 100%;
  overflow: hidden;
}

.model-evaluation-preview__frame {
  position: absolute;
  top: 50%;
  left: 50%;
  width: 960px;
  height: 600px;
  max-width: none;
  border: 0;
  transform-origin: center;
  /* This is an animation preview; gallery controls receive pointer/focus input. */
  pointer-events: none;
}
</style>
