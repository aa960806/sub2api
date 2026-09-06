<script setup lang="ts">
import { computed } from 'vue'

type GlassTheme = 'cool-slate' | 'deep-night' | 'cyan-mist'

interface GlassPaneProps {
  theme?: GlassTheme
}

const props = withDefaults(defineProps<GlassPaneProps>(), {
  theme: 'deep-night' as GlassTheme,
})

const themeStyle = computed(() => {
  const backgrounds: Record<GlassTheme, string> = {
    'cool-slate': 'rgba(6, 14, 27, 0.30)',
    'deep-night': 'rgba(4, 8, 19, 0.32)',
    'cyan-mist': 'rgba(4, 17, 26, 0.29)',
  }
  const borders: Record<GlassTheme, string> = {
    'cool-slate': 'rgba(255, 255, 255, 0.32)',
    'deep-night': 'rgba(255, 255, 255, 0.30)',
    'cyan-mist': 'rgba(103, 232, 249, 0.34)',
  }

  return {
    backgroundColor: backgrounds[props.theme],
    borderColor: borders[props.theme],
    backdropFilter: 'blur(14px) saturate(135%)',
    WebkitBackdropFilter: 'blur(14px) saturate(135%)',
    boxShadow: '0 24px 70px -12px rgba(1, 4, 12, 0.62), inset 0 1px 1px 0 rgba(255, 255, 255, 0.24), inset 0 0 0 1px rgba(255, 255, 255, 0.14), inset 0 -1px 2px 0 rgba(0, 0, 0, 0.3)',
  }
})
</script>

<template>
  <div class="relative flex min-h-screen w-full select-none items-center justify-center overflow-x-hidden p-0 text-white md:p-5 lg:p-7">
    <main
      class="glass-pane relative flex min-h-[100svh] w-full max-w-[1720px] flex-1 flex-col justify-between overflow-y-auto border-0 transition-all duration-500 ease-out md:min-h-[calc(100svh-2.5rem)] md:rounded-[2.25rem] md:border md:border-solid lg:min-h-[calc(100svh-3.5rem)]"
      :style="themeStyle"
    >
      <div class="pointer-events-none absolute inset-0 z-10 hidden rounded-[2.25rem] border border-white/[0.2] shadow-[inset_0_0_0_1px_rgba(167,243,255,0.1)] md:block" />
      <div class="pointer-events-none absolute left-[6%] right-[6%] top-0 z-10 hidden h-px bg-gradient-to-r from-transparent via-white/65 to-transparent md:block" />
      <div class="pointer-events-none absolute bottom-0 left-[8%] right-[8%] z-10 hidden h-px bg-gradient-to-r from-transparent via-cyan-100/25 to-transparent md:block" />
      <div class="pointer-events-none absolute bottom-[8%] left-0 top-[8%] z-10 hidden w-px bg-gradient-to-b from-transparent via-white/38 to-transparent md:block" />
      <div class="pointer-events-none absolute bottom-[8%] right-0 top-[8%] z-10 hidden w-px bg-gradient-to-b from-transparent via-white/32 to-transparent md:block" />

      <div class="relative z-20 flex w-full flex-1 flex-col justify-between">
        <slot />
      </div>
    </main>
  </div>
</template>

<style scoped>
.glass-pane {
  -ms-overflow-style: none;
  scrollbar-width: none;
}

.glass-pane::-webkit-scrollbar {
  display: none;
}
</style>
