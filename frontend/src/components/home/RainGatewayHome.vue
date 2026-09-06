<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import LocaleSwitcher from '@/components/common/LocaleSwitcher.vue'
import RainGlyph from './RainGlyph.vue'
import RainyBackground from './RainyBackground.vue'
import GlassPane from './GlassPane.vue'

interface RainGatewayHomeProps {
  siteName: string
  siteLogo: string
  siteSubtitle: string
  docUrl: string
  showModelPlazaEntry: boolean
  isAuthenticated: boolean
  dashboardPath: string
  userInitial: string
  isDark: boolean
  currentYear: number
  /** The existing home page has no animation toggle; this is kept as a host prop for reduced-motion handling. */
  animationsEnabled?: boolean
}

const props = withDefaults(defineProps<RainGatewayHomeProps>(), {
  animationsEnabled: true
})

const emit = defineEmits<{
  'toggle-theme': []
}>()

const { t } = useI18n()

const mouseX = ref(typeof window === 'undefined' ? 0 : window.innerWidth / 2)
const mouseY = ref(typeof window === 'undefined' ? 0 : window.innerHeight / 2)

const destination = computed(() => props.isAuthenticated ? props.dashboardPath : '/login')
const brandTitle = computed(() => {
  const match = props.siteName.match(/^(.*?)(api)$/i)
  return match
    ? { base: match[1], accent: match[2] }
    : { base: props.siteName, accent: '' }
})
const gatewayLabel = computed(() => {
  const value = props.siteName.trim()
  return value ? value.toUpperCase() : 'API GATEWAY'
})
const brandDescriptor = computed(() => props.siteSubtitle.trim() || t('home.providers.description'))
const animationActive = computed(() => props.animationsEnabled)

const featureCards = [
  { icon: 'gauge', title: 'home.features.unifiedGateway', description: 'home.features.unifiedGatewayDesc', color: 'amber' },
  { icon: 'shield-check', title: 'home.features.multiAccount', description: 'home.features.multiAccountDesc', color: 'cyan' },
  { icon: 'waves', title: 'home.features.balanceQuota', description: 'home.features.balanceQuotaDesc', color: 'emerald' }
] as const

const featureColorClass = {
  amber: 'text-amber-200',
  cyan: 'text-cyan-200',
  emerald: 'text-emerald-200'
} as const

const providers = [
  { key: 'claude', label: 'home.providers.claude', letter: 'C', color: 'orange', soon: false },
  { key: 'gpt', label: 'GPT', letter: 'G', color: 'green', soon: false },
  { key: 'gemini', label: 'home.providers.gemini', letter: 'G', color: 'blue', soon: false },
  { key: 'antigravity', label: 'home.providers.antigravity', letter: 'A', color: 'rose', soon: false },
  { key: 'more', label: 'home.providers.more', letter: '+', color: 'slate', soon: true }
] as const

const chartBars = [28, 40, 32, 52, 44, 64, 48, 70, 58, 78, 62, 86, 72, 94, 78, 100, 88, 96]

function handleMouseMove(event: MouseEvent) {
  mouseX.value = event.clientX
  mouseY.value = event.clientY
}

onMounted(() => {
  window.addEventListener('mousemove', handleMouseMove, { passive: true })
})

onBeforeUnmount(() => {
  window.removeEventListener('mousemove', handleMouseMove)
})
</script>

<template>
  <div
    class="rain-gateway-root relative min-h-screen w-full select-none text-white antialiased"
    :class="{ 'contrast-[0.95] brightness-[1.1]': !isDark }"
  >
    <RainyBackground
      :enabled="animationActive"
      :image-index="2"
      :mouse-x="mouseX"
      :mouse-y="mouseY"
      theme="deep-night"
    />

    <GlassPane theme="deep-night">
      <!-- Target page navigation, with the existing locale, docs, model-plaza, theme and auth bindings. -->
      <header class="rain-gateway-nav flex w-full items-center justify-between px-4 py-4 sm:px-16 sm:py-6 lg:px-24">
        <div class="rain-gateway-brand flex min-w-0 cursor-pointer items-center gap-3">
          <div class="rain-gateway-mark relative flex h-10 w-10 shrink-0 items-center justify-center rounded-2xl border border-cyan-100/25 bg-slate-950/60 shadow-lg backdrop-blur-xl transition-colors">
            <img
              :src="siteLogo || '/logo.svg'"
              :alt="siteName"
              class="h-5 w-5 object-contain"
              draggable="false"
            />
          </div>
          <div class="hidden min-w-0 sm:block">
            <div class="truncate text-sm font-semibold tracking-[0.12em] text-white">{{ siteName }}</div>
            <div class="max-w-[15rem] truncate text-[10px] uppercase tracking-[0.2em] text-white/40">{{ brandDescriptor }}</div>
          </div>
        </div>

        <div class="rain-gateway-controls flex max-w-full items-center gap-1 sm:gap-5">
          <LocaleSwitcher variant="gateway" class="rain-gateway-locale" />

          <a
            v-if="docUrl"
            :href="docUrl"
            target="_blank"
            rel="noopener noreferrer"
            class="rain-gateway-icon-button"
            :title="t('home.viewDocs')"
            :aria-label="t('home.viewDocs')"
          >
            <RainGlyph name="cloud-rain" size="sm" />
          </a>

          <router-link
            v-if="showModelPlazaEntry"
            to="/model-plaza"
            class="rain-gateway-icon-button"
            :title="t('nav.modelPlaza')"
            :aria-label="t('nav.modelPlaza')"
          >
            <RainGlyph name="waves" size="sm" />
          </router-link>

          <button
            type="button"
            class="rain-gateway-icon-button"
            :title="isDark ? t('home.switchToLight') : t('home.switchToDark')"
            :aria-label="isDark ? t('home.switchToLight') : t('home.switchToDark')"
            @click="emit('toggle-theme')"
          >
            <RainGlyph :name="isDark ? 'sun' : 'moon'" size="sm" />
          </button>

          <router-link
            :to="destination"
            class="rain-gateway-console group inline-flex min-h-9 items-center gap-1.5 rounded-full px-2 sm:gap-2 sm:px-3.5 sm:py-1.5"
          >
            <span class="rain-gateway-console-badge">{{ isAuthenticated ? (userInitial || '•') : '→' }}</span>
            <span class="hidden text-xs font-medium text-white/95 sm:inline">{{ isAuthenticated ? t('home.dashboard') : t('home.login') }}</span>
            <RainGlyph name="arrow-right" class="rain-gateway-console-arrow text-teal-300 transition-transform group-hover:translate-x-0.5" />
          </router-link>
        </div>
      </header>

      <section class="rain-gateway-hero grid w-full flex-1 items-center gap-12 px-5 py-10 sm:px-16 sm:py-12 lg:grid-cols-[minmax(0,1fr)_minmax(340px,0.72fr)] lg:gap-12 lg:px-24 lg:py-20">
        <div class="rain-gateway-copy max-w-3xl">
          <div class="mb-5 flex items-center gap-3">
            <span class="h-px w-8 bg-cyan-300/80" />
            <span class="text-[11px] font-semibold uppercase tracking-[0.24em] text-cyan-200/90 sm:text-xs">{{ gatewayLabel }}</span>
          </div>

          <h1 class="rain-gateway-title mb-5 text-5xl font-semibold text-white sm:text-7xl lg:text-8xl">
            {{ brandTitle.base }}<span v-if="brandTitle.accent" class="text-cyan-200">{{ brandTitle.accent }}</span>
          </h1>
          <p class="rain-gateway-subtitle mb-5 max-w-xl text-xl font-light leading-tight text-white/95 sm:text-2xl lg:text-[2rem]">{{ siteSubtitle }}</p>
          <p class="rain-gateway-description mb-8 max-w-lg text-sm font-normal leading-relaxed text-white/60 sm:text-base">{{ t('home.heroDescription') }}</p>

          <div class="flex flex-wrap items-center gap-3">
            <router-link
              :to="destination"
              class="rain-gateway-primary group inline-flex items-center gap-2.5 rounded-xl px-5 py-3 text-sm font-semibold"
            >
              <span>{{ isAuthenticated ? t('home.goToDashboard') : t('home.getStarted') }}</span>
              <RainGlyph name="arrow-right" size="sm" class="transition-transform group-hover:translate-x-1" />
            </router-link>
            <span class="rain-gateway-endpoint inline-flex items-center gap-2 rounded-xl px-3.5 py-3 text-xs text-white/60 backdrop-blur-md">
              <code class="max-w-full truncate font-mono text-white/80">POST /v1/messages</code>
            </span>
          </div>
        </div>

        <!-- The target status panel remains decorative; its labels use current settings/i18n instead of demo metrics. -->
        <div class="rain-gateway-status-wrap terminal-container relative hidden lg:block">
          <div class="rain-gateway-status relative overflow-hidden rounded-[1.75rem] border border-white/20 bg-slate-950/35 p-5 shadow-[0_24px_60px_rgba(0,0,0,0.3)] backdrop-blur-xl">
            <div class="mb-8 flex items-center justify-between text-xs text-white/55">
              <span class="flex items-center gap-2"><RainGlyph name="cloud-rain" size="sm" class="text-cyan-200" /> {{ t('home.providers.title') }}</span>
              <span class="flex items-center gap-1.5 text-emerald-300"><span class="h-1.5 w-1.5 rounded-full bg-emerald-300 shadow-[0_0_10px_currentColor]" /> {{ isAuthenticated ? t('home.dashboard') : t('home.login') }}</span>
            </div>
            <div class="mb-7 flex items-end justify-between gap-4">
              <div class="min-w-0">
                <div class="mb-1 text-[11px] uppercase tracking-[0.18em] text-white/40">{{ t('home.providers.description') }}</div>
                <div class="rain-gateway-status-value truncate font-mono text-4xl font-light tracking-tight text-white">{{ siteName }}</div>
              </div>
              <span class="rain-gateway-status-chip flex shrink-0 items-center gap-1.5 rounded-full border border-emerald-300/20 bg-emerald-300/10 px-2.5 py-1 text-[11px] text-emerald-200">
                <RainGlyph name="activity" class="rain-gateway-activity-icon" />
                {{ t('home.providers.supported') }}
              </span>
            </div>
            <div class="mb-6 flex h-16 items-end gap-1.5 opacity-80" aria-hidden="true">
              <span v-for="(height, index) in chartBars" :key="index" class="flex-1 rounded-t bg-gradient-to-t from-cyan-300/20 to-cyan-100/75" :style="{ height: `${height}%` }" />
            </div>
            <div class="grid grid-cols-2 gap-4 border-t border-white/10 pt-4">
              <div class="min-w-0">
                <div class="mb-1 flex items-center gap-1.5 text-[11px] text-white/45"><RainGlyph name="activity" size="xs" /> {{ t('home.tags.subscriptionToApi') }}</div>
                <div class="truncate text-sm font-medium text-white">{{ t('home.tags.stickySession') }}</div>
              </div>
              <div class="min-w-0">
                <div class="mb-1 flex items-center gap-1.5 text-[11px] text-white/45"><RainGlyph name="check" size="xs" /> {{ t('home.tags.realtimeBilling') }}</div>
                <div class="truncate text-sm font-medium text-white">{{ siteSubtitle }}</div>
              </div>
            </div>
          </div>
        </div>
      </section>

      <section class="rain-gateway-features w-full px-8 pb-8 sm:px-16 sm:pb-12 lg:px-24">
        <div class="rain-gateway-feature-grid grid max-w-5xl grid-cols-1 gap-px overflow-hidden rounded-2xl border border-white/[0.12] bg-white/10 backdrop-blur-md md:grid-cols-3">
          <article
            v-for="feature in featureCards"
            :key="feature.title"
            class="rain-gateway-feature-card group p-4 transition-all duration-300 sm:p-5"
          >
            <div class="mb-2.5">
              <RainGlyph :name="feature.icon" size="sm" class="transition-transform group-hover:scale-105" :class="featureColorClass[feature.color]" />
            </div>
            <h2 class="mb-1.5 text-sm font-semibold tracking-tight text-white/95 sm:text-base">{{ t(feature.title) }}</h2>
            <p class="text-xs font-normal leading-relaxed text-white/60">{{ t(feature.description) }}</p>
          </article>
        </div>
      </section>

    </GlassPane>

      <!-- Existing provider display remains available below the target fold, with the target glass treatment. -->
      <div class="rain-gateway-tail">
      <section class="rain-gateway-providers w-full px-8 pb-10 pt-10 sm:px-16 lg:px-24">
        <div class="mb-5 text-center">
          <h2 class="text-lg font-semibold text-white/90">{{ t('home.providers.title') }}</h2>
          <p class="mt-1 text-xs text-white/55">{{ t('home.providers.description') }}</p>
        </div>
        <div class="flex flex-wrap items-center justify-center gap-3">
          <div
            v-for="provider in providers"
            :key="provider.key"
            class="rain-gateway-provider inline-flex items-center gap-2 rounded-xl border px-4 py-2.5 backdrop-blur-md"
            :class="provider.soon ? 'rain-gateway-provider-muted' : ''"
          >
            <span class="rain-gateway-provider-mark" :class="`rain-gateway-provider-${provider.color}`">{{ provider.letter }}</span>
            <span class="text-xs font-medium text-white/80">{{ provider.label.startsWith('home.') ? t(provider.label) : provider.label }}</span>
            <span class="rounded bg-cyan-300/10 px-1.5 py-0.5 text-[10px] font-medium text-cyan-200/80">{{ t(provider.soon ? 'home.providers.soon' : 'home.providers.supported') }}</span>
          </div>
        </div>
      </section>

      <footer class="rain-gateway-footer flex w-full flex-col items-center justify-center gap-3 border-t border-white/10 px-6 py-6 text-center text-xs text-white/45 sm:flex-row sm:justify-between sm:px-16 lg:px-24">
        <p>&copy; {{ currentYear }} {{ siteName }}. {{ t('home.footer.allRightsReserved') }}</p>
        <div class="flex items-center gap-4">
          <a v-if="docUrl" :href="docUrl" target="_blank" rel="noopener noreferrer" class="transition-colors hover:text-white">{{ t('home.docs') }}</a>
          <a href="https://github.com/Wei-Shaw/sub2api" target="_blank" rel="noopener noreferrer" class="transition-colors hover:text-white">GitHub</a>
        </div>
      </footer>
      </div>
  </div>
</template>

<style scoped>
.rain-gateway-root {
  min-height: 100svh;
  overflow-x: hidden;
  background: #040812;
}

.rain-gateway-nav,
.rain-gateway-hero,
.rain-gateway-features,
.rain-gateway-providers,
.rain-gateway-footer {
  position: relative;
  z-index: 20;
}

.rain-gateway-nav {
  z-index: 30;
}

.rain-gateway-mark {
  border-color: rgba(207, 250, 254, 0.25);
  background: rgba(2, 10, 20, 0.6);
}

.rain-gateway-mark:hover {
  border-color: rgba(207, 250, 254, 0.5);
}

.rain-gateway-mark img {
  filter: drop-shadow(0 0 8px rgba(103, 232, 249, 0.35));
}

.rain-gateway-controls {
  flex-wrap: wrap;
  justify-content: flex-end;
}

.rain-gateway-icon-button {
  display: inline-flex;
  min-width: 2.25rem;
  min-height: 2.25rem;
  align-items: center;
  justify-content: center;
  border-radius: 9999px;
  color: rgba(255, 255, 255, 0.54);
  transition: color 0.2s ease, background 0.2s ease, border-color 0.2s ease;
}

.rain-gateway-icon-button:hover,
.rain-gateway-icon-button:focus-visible {
  color: #fff;
  outline: none;
}

.rain-gateway-console {
  border: 1px solid rgba(14, 72, 68, 0.8);
  background: rgba(5, 32, 38, 0.7);
  color: #fff;
  box-shadow: 0 4px 16px rgba(0, 0, 0, 0.12);
  backdrop-filter: blur(14px);
  -webkit-backdrop-filter: blur(14px);
  transition: border-color 0.2s ease, transform 0.2s ease;
}

.rain-gateway-console:hover,
.rain-gateway-console:focus-visible {
  border-color: #14b8a6;
  outline: 2px solid rgba(20, 184, 166, 0.4);
  outline-offset: 2px;
}

.rain-gateway-console:active {
  transform: scale(0.95);
}

.rain-gateway-console-arrow,
.rain-gateway-activity-icon {
  width: 0.875rem;
  height: 0.875rem;
}

.rain-gateway-console-badge {
  display: inline-flex;
  width: 1rem;
  height: 1rem;
  align-items: center;
  justify-content: center;
  border-radius: 9999px;
  background: #0f766e;
  color: #ccfbf1;
  font-size: 0.625rem;
  font-weight: 700;
}

.rain-gateway-title {
  max-width: 100%;
  overflow-wrap: anywhere;
  line-height: 0.98;
  letter-spacing: 0;
  text-shadow: 0 8px 30px rgba(0, 0, 0, 0.3);
}

.rain-gateway-copy p,
.rain-gateway-status,
.rain-gateway-feature-card,
.rain-gateway-provider,
.rain-gateway-endpoint {
  overflow-wrap: anywhere;
}

.rain-gateway-primary {
  background: #67e8f9;
  color: #020617;
  box-shadow: 0 8px 24px rgba(34, 211, 238, 0.18);
  transition: background 0.3s ease, transform 0.3s ease, box-shadow 0.3s ease;
}

.rain-gateway-primary:hover,
.rain-gateway-primary:focus-visible {
  background: #a5f3fc;
  box-shadow: 0 12px 32px rgba(34, 211, 238, 0.28);
  outline: 2px solid rgba(165, 243, 252, 0.5);
  outline-offset: 2px;
}

.rain-gateway-primary:active {
  transform: scale(0.95);
}

.rain-gateway-endpoint {
  border: 1px solid rgba(255, 255, 255, 0.15);
  background: rgba(255, 255, 255, 0.07);
}

.rain-gateway-status {
  border-color: rgba(255, 255, 255, 0.2);
  background: rgba(2, 6, 23, 0.35);
  box-shadow: 0 24px 60px rgba(0, 0, 0, 0.3);
}

.rain-gateway-status-value {
  max-width: 18rem;
  overflow-wrap: anywhere;
}

.rain-gateway-status-chip {
  white-space: nowrap;
}

.rain-gateway-feature-grid {
  box-shadow: 0 20px 48px rgba(0, 0, 0, 0.16);
}

.rain-gateway-feature-card {
  background: rgba(2, 6, 23, 0.35);
}

.rain-gateway-feature-card:hover {
  background: rgba(255, 255, 255, 0.08);
}

.rain-gateway-provider {
  border-color: rgba(255, 255, 255, 0.14);
  background: rgba(2, 6, 23, 0.32);
}

.rain-gateway-provider-muted {
  opacity: 0.58;
}

.rain-gateway-provider-mark {
  display: inline-flex;
  width: 1.5rem;
  height: 1.5rem;
  align-items: center;
  justify-content: center;
  border-radius: 0.5rem;
  color: #fff;
  font-size: 0.7rem;
  font-weight: 700;
}

.rain-gateway-provider-orange { background: linear-gradient(135deg, #fb923c, #f97316); }
.rain-gateway-provider-green { background: linear-gradient(135deg, #22c55e, #16a34a); }
.rain-gateway-provider-blue { background: linear-gradient(135deg, #3b82f6, #2563eb); }
.rain-gateway-provider-rose { background: linear-gradient(135deg, #f43f5e, #db2777); }
.rain-gateway-provider-slate { background: linear-gradient(135deg, #64748b, #475569); }

.rain-gateway-footer {
  background: rgba(3, 12, 21, 0.22);
  backdrop-filter: blur(14px);
  -webkit-backdrop-filter: blur(14px);
}

@media (max-width: 639px) {
  .rain-gateway-nav {
    padding-left: 1rem;
    padding-right: 1rem;
  }

  .rain-gateway-controls {
    gap: 0;
  }

  .rain-gateway-icon-button {
    min-width: 2rem;
    min-height: 2rem;
  }

}

@media (prefers-reduced-motion: reduce) {
  .rain-gateway-root,
  .rain-gateway-root :deep(*),
  .rain-gateway-root :deep(*::before),
  .rain-gateway-root :deep(*::after) {
    animation-duration: 0.01ms !important;
    animation-iteration-count: 1 !important;
    transition-duration: 0.01ms !important;
    scroll-behavior: auto !important;
  }
}
</style>
