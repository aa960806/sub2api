<template>
  <div
    v-if="currentMessage"
    class="pointer-events-none fixed left-1/2 top-3 z-[100000] w-[min(820px,calc(100vw-24px))] -translate-x-1/2"
    data-testid="broadcast-marquee"
  >
    <div class="pointer-events-auto flex min-h-12 items-center gap-3 overflow-hidden rounded-md border border-amber-200 bg-white/95 px-3 py-2 shadow-lg shadow-amber-900/10 backdrop-blur dark:border-amber-400/30 dark:bg-dark-900/95">
      <div class="flex h-8 w-8 shrink-0 items-center justify-center rounded-md bg-amber-100 text-amber-700 dark:bg-amber-400/15 dark:text-amber-300">
        <Icon name="bell" size="sm" aria-hidden="true" />
      </div>
      <div class="min-w-0 flex-1 overflow-hidden">
        <div
          :key="currentMessageKey"
          class="marquee-track whitespace-nowrap text-sm font-medium text-gray-800 dark:text-dark-100"
          :style="{ animationDuration: `${animationDuration}s` }"
          @animationend="advance"
        >
          <span class="inline-flex items-center gap-2">
            <span v-if="currentMessage.title" class="font-semibold text-amber-700 dark:text-amber-300">{{ currentMessage.title }}</span>
            <span>{{ currentMessage.content }}</span>
          </span>
        </div>
      </div>
      <button
        type="button"
        class="btn-icon shrink-0 text-gray-400 hover:text-gray-700 dark:text-dark-400 dark:hover:text-dark-100"
        :aria-label="t('marquee.dismiss')"
        :title="t('marquee.dismiss')"
        @click="dismiss"
      >
        <Icon name="x" size="sm" aria-hidden="true" />
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import { listActiveMarqueeBroadcasts, type MarqueeBroadcast } from '@/api/marquee'
import { useAppStore } from '@/stores/app'
import { useAuthStore } from '@/stores/auth'

const { t } = useI18n()
const appStore = useAppStore()
const authStore = useAuthStore()
const messages = ref<MarqueeBroadcast[]>([])
const seenKeys = ref<Set<string>>(new Set())
const currentIndex = ref(0)
const cycleCount = ref(0)
let timer: number | undefined
let requestGeneration = 0

const featureEnabled = computed(
  () => appStore.publicSettingsLoaded && appStore.cachedPublicSettings?.subnexus_marquee_enabled === true,
)
const pollingAllowed = computed(() => authStore.isAuthenticated && featureEnabled.value)
const storageKey = computed(() => `sub2api:marquee:seen:${authStore.user?.id ?? 'guest'}`)
const visibleMessages = computed(() => messages.value.filter((item) => !seenKeys.value.has(messageKey(item))))
const currentMessage = computed(() => {
  const list = visibleMessages.value
  if (!list.length) return null
  return list[currentIndex.value % list.length] ?? list[0]
})
const currentMessageKey = computed(() => currentMessage.value
  ? `${messageKey(currentMessage.value)}:${cycleCount.value}`
  : '')
const animationDuration = computed(() => {
  const item = currentMessage.value
  const textLength = item ? item.title.length + item.content.length : 0
  return Math.min(42, Math.max(16, Math.ceil(textLength / 6)))
})

function messageKey(item: MarqueeBroadcast): string {
  return `${item.id}:${item.updated_at || item.created_at || ''}`
}

function loadSeenKeys(): void {
  try {
    const parsed = JSON.parse(window.localStorage.getItem(storageKey.value) || '[]')
    seenKeys.value = new Set(Array.isArray(parsed) ? parsed.map(String) : [])
  } catch {
    seenKeys.value = new Set()
  }
}

function saveSeenKeys(): void {
  try {
    const values = Array.from(seenKeys.value).slice(-200)
    window.localStorage.setItem(storageKey.value, JSON.stringify(values))
    seenKeys.value = new Set(values)
  } catch {
    // Storage is optional; fail closed only affects network access and display.
  }
}

async function loadBroadcasts(): Promise<void> {
  if (!pollingAllowed.value || document.visibilityState === 'hidden') return
  const generation = ++requestGeneration
  try {
    const result = await listActiveMarqueeBroadcasts(12)
    if (generation !== requestGeneration || !pollingAllowed.value) return
    if (result?.enabled !== true) {
      disableLocalRuntime()
      return
    }
    messages.value = (result.items ?? []).filter(
      (item) => item.source === 'admin' && !seenKeys.value.has(messageKey(item)),
    )
  } catch {
    if (generation === requestGeneration) disableLocalRuntime()
  }
}

function disableLocalRuntime(): void {
  if (appStore.cachedPublicSettings) {
    appStore.cachedPublicSettings = {
      ...appStore.cachedPublicSettings,
      subnexus_marquee_enabled: false,
    }
  }
  stopPolling()
  messages.value = []
}

function startPolling(): void {
  stopPolling()
  void loadBroadcasts()
  timer = window.setInterval(() => { void loadBroadcasts() }, 30_000)
}

function stopPolling(): void {
  requestGeneration += 1
  if (timer !== undefined) {
    window.clearInterval(timer)
    timer = undefined
  }
}

function dismiss(): void {
  const next = new Set(seenKeys.value)
  for (const item of visibleMessages.value) next.add(messageKey(item))
  seenKeys.value = next
  saveSeenKeys()
  messages.value = []
}

function advance(): void {
  if (!visibleMessages.value.length) return
  currentIndex.value = (currentIndex.value + 1) % visibleMessages.value.length
  cycleCount.value += 1
}

function onVisibilityChange(): void {
  if (document.visibilityState === 'visible' && pollingAllowed.value) void loadBroadcasts()
}

watch(storageKey, () => {
  loadSeenKeys()
  messages.value = []
}, { immediate: true })

watch(pollingAllowed, (allowed) => {
  if (allowed) startPolling()
  else {
    stopPolling()
    messages.value = []
  }
}, { immediate: true })

document.addEventListener('visibilitychange', onVisibilityChange)
onBeforeUnmount(() => {
  stopPolling()
  document.removeEventListener('visibilitychange', onVisibilityChange)
})
</script>

<style scoped>
.marquee-track {
  display: inline-block;
  min-width: 100%;
  animation-name: subnexus-marquee;
  animation-timing-function: linear;
  animation-iteration-count: 1;
  animation-fill-mode: forwards;
}

.marquee-track:hover {
  animation-play-state: paused;
}

@keyframes subnexus-marquee {
  0% { transform: translateX(100%); }
  100% { transform: translateX(-100%); }
}
</style>
