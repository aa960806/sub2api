<template>
  <template v-if="isEnabled">
    <div
      :class="isRainHome ? 'fixed right-6 z-40 select-none sm:right-8' : ''"
      :style="isRainHome ? { bottom: 'max(1.5rem, env(safe-area-inset-bottom))' } : undefined"
    >
      <button
        :class="isRainHome
          ? 'rain-support-button group relative flex h-12 w-12 cursor-pointer items-center justify-center rounded-full bg-[#06b6d4] text-white shadow-[0_4px_25px_rgba(6,182,212,0.45)] transition-all duration-300 hover:bg-[#0891b2] hover:shadow-[0_4px_35px_rgba(6,182,212,0.7)] focus:outline-none focus:ring-4 focus:ring-cyan-300/40 active:scale-90 sm:h-14 sm:w-14'
          : 'fixed bottom-5 right-5 z-[110] flex h-12 w-12 items-center justify-center rounded-full bg-primary-600 text-white shadow-lg transition hover:bg-primary-700 focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2 dark:focus:ring-offset-dark-900'"
        type="button"
        :aria-label="t('common.contactSupport')"
        :title="t('common.contactSupport')"
        data-testid="customer-support-button"
        @click="open"
      >
        <RainGlyph v-if="isRainHome" name="headphones" size="lg" class="transition-transform group-hover:scale-110" />
        <Icon v-else name="chatBubble" size="md" aria-hidden="true" />
        <template v-if="isRainHome">
          <span class="absolute -right-1 -top-1 h-3.5 w-3.5 animate-ping rounded-full bg-emerald-400 ring-2 ring-slate-900" />
          <span class="absolute -right-1 -top-1 h-3.5 w-3.5 rounded-full bg-emerald-400 ring-2 ring-slate-900" />
        </template>
      </button>
    </div>
    <CustomerSupportModal v-model:visible="visible" />
  </template>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute } from 'vue-router'
import Icon from '@/components/icons/Icon.vue'
import RainGlyph from '@/components/home/RainGlyph.vue'
import CustomerSupportModal from './CustomerSupportModal.vue'
import { useAppStore } from '@/stores'

const { t } = useI18n()
const route = useRoute()
const appStore = useAppStore()
const visible = ref(false)

const isEnabled = computed(() => (
  appStore.publicSettingsLoaded
  && appStore.cachedPublicSettings?.customer_support_enabled === true
  && (appStore.cachedPublicSettings?.customer_support_content || '').trim().length > 0
))

const isRainHome = computed(() => (
  route.name === 'Home'
  && !(appStore.cachedPublicSettings?.home_content || '').trim()
  && appStore.cachedPublicSettings?.compact_home_enabled !== true
))

function open() {
  if (isEnabled.value) visible.value = true
}

watch(isEnabled, (enabled) => {
  if (!enabled) visible.value = false
})
</script>

<style scoped>
@media (prefers-reduced-motion: reduce) {
  .rain-support-button,
  .rain-support-button * {
    animation-duration: 0.01ms !important;
    animation-iteration-count: 1 !important;
    transition-duration: 0.01ms !important;
  }
}
</style>
