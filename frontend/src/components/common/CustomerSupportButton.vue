<template>
  <Teleport to="body">
    <div
      v-if="isEnabled"
      :class="isRainHome ? 'fixed right-6 z-40 select-none sm:right-8' : ['customer-support-button', { 'is-open': isModalOpen }]"
      :style="isRainHome ? { bottom: 'max(1.5rem, env(safe-area-inset-bottom))' } : undefined"
    >
      <button
        type="button"
        :class="isRainHome
          ? 'rain-support-button group relative flex h-12 w-12 cursor-pointer items-center justify-center rounded-full bg-[#06b6d4] text-white shadow-[0_4px_25px_rgba(6,182,212,0.45)] transition-all duration-300 hover:bg-[#0891b2] hover:shadow-[0_4px_35px_rgba(6,182,212,0.7)] focus:outline-none focus:ring-4 focus:ring-cyan-300/40 active:scale-90 sm:h-14 sm:w-14'
          : 'support-btn'"
        :aria-label="t('common.contactSupport')"
        :title="t('common.contactSupport')"
        data-testid="customer-support-button"
        @click="toggleModal"
      >
        <RainGlyph v-if="isRainHome" name="headphones" size="lg" class="transition-transform group-hover:scale-110" />
        <svg
          v-else
          class="support-icon"
          xmlns="http://www.w3.org/2000/svg"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          stroke-width="2"
          stroke-linecap="round"
          stroke-linejoin="round"
        >
          <path d="M3 11h3a2 2 0 0 1 2 2v3a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-5Zm0 0a9 9 0 1 1 18 0m0 0v5a2 2 0 0 1-2 2h-1a2 2 0 0 1-2-2v-3a2 2 0 0 1 2-2h3Z" />
        </svg>
        <template v-if="isRainHome">
          <span class="absolute -right-1 -top-1 h-3.5 w-3.5 animate-ping rounded-full bg-emerald-400 ring-2 ring-slate-900" />
          <span class="absolute -right-1 -top-1 h-3.5 w-3.5 rounded-full bg-emerald-400 ring-2 ring-slate-900" />
        </template>
      </button>
    </div>

    <CustomerSupportModal v-if="isEnabled" v-model:visible="isModalOpen" />
  </Teleport>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute } from 'vue-router'
import RainGlyph from '@/components/home/RainGlyph.vue'
import { useAppStore } from '@/stores'
import CustomerSupportModal from './CustomerSupportModal.vue'

const { t } = useI18n()
const route = useRoute()
const appStore = useAppStore()
const isModalOpen = ref(false)

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

function toggleModal() {
  if (isEnabled.value) isModalOpen.value = !isModalOpen.value
}

watch(isEnabled, (enabled) => {
  if (!enabled) isModalOpen.value = false
})
</script>

<style scoped>
.customer-support-button {
  position: fixed;
  right: 24px;
  bottom: 24px;
  z-index: 9999;
}

.support-btn {
  position: relative;
  display: flex;
  align-items: center;
  justify-content: center;
  width: 56px;
  height: 56px;
  padding: 0;
  color: #ffffff;
  background: linear-gradient(135deg, #14b8a6 0%, #0ea5e9 100%);
  border: none;
  border-radius: 50%;
  box-shadow:
    0 8px 24px -6px rgba(14, 165, 233, 0.5),
    0 2px 6px rgba(15, 23, 42, 0.12);
  cursor: pointer;
  transition:
    transform 0.25s cubic-bezier(0.34, 1.56, 0.64, 1),
    box-shadow 0.25s ease;
}

/* 呼吸光环提示 */
.support-btn::before {
  content: '';
  position: absolute;
  inset: 0;
  border-radius: 50%;
  background: linear-gradient(135deg, #14b8a6 0%, #0ea5e9 100%);
  opacity: 0.55;
  z-index: -1;
  animation: support-pulse 2.4s ease-out infinite;
}

@keyframes support-pulse {
  0% {
    transform: scale(1);
    opacity: 0.55;
  }
  70% {
    transform: scale(1.55);
    opacity: 0;
  }
  100% {
    transform: scale(1.55);
    opacity: 0;
  }
}

.support-btn:hover {
  transform: translateY(-3px) scale(1.06);
  box-shadow:
    0 12px 30px -6px rgba(14, 165, 233, 0.6),
    0 4px 10px rgba(15, 23, 42, 0.18);
}

.support-btn:active {
  transform: translateY(-1px) scale(0.97);
}

.support-icon {
  width: 28px;
  height: 28px;
  transition: transform 0.3s ease;
}

.is-open .support-btn {
  background: linear-gradient(135deg, #0d9488 0%, #0284c7 100%);
}

/* 弹窗打开后停止呼吸动画 */
.is-open .support-btn::before {
  animation: none;
  opacity: 0;
}

.is-open .support-icon {
  transform: scale(0.9);
}

/* 尊重用户的减少动画偏好 */
@media (prefers-reduced-motion: reduce) {
  .support-btn::before {
    animation: none;
    opacity: 0;
  }

  .rain-support-button,
  .rain-support-button * {
    animation-duration: 0.01ms !important;
    animation-iteration-count: 1 !important;
    transition-duration: 0.01ms !important;
  }
}

/* 响应式设计 */
@media (max-width: 768px) {
  .customer-support-button {
    right: 16px;
    bottom: 16px;
  }

  .support-btn {
    width: 52px;
    height: 52px;
  }

  .support-icon {
    width: 24px;
    height: 24px;
  }
}

/* 暗色模式支持 */
html.dark .support-btn {
  background: linear-gradient(135deg, #14b8a6 0%, #0ea5e9 100%);
}

html.dark .support-btn:hover {
  box-shadow:
    0 6px 20px rgba(20, 184, 166, 0.5),
    0 4px 8px rgba(0, 0, 0, 0.3);
}
</style>
