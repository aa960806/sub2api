<template>
  <div class="relative" ref="dropdownRef">
    <button
      type="button"
      @click="toggleDropdown"
      :disabled="switching"
      class="flex items-center transition-colors"
      :class="props.variant === 'gateway'
        ? 'min-h-9 cursor-pointer gap-1 px-0 text-xs font-normal text-white/75 hover:text-white focus:outline-none'
        : 'gap-1.5 rounded-lg px-2 py-1.5 text-sm font-medium text-gray-600 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-dark-700'"
      :title="currentLocale?.name"
    >
      <span v-if="props.variant === 'gateway'" class="whitespace-nowrap text-xs font-normal">{{ gatewayLabel }}</span>
      <template v-else>
        <span class="text-base">{{ currentLocale?.flag }}</span>
        <span class="hidden sm:inline">{{ currentLocale?.code.toUpperCase() }}</span>
      </template>
      <Icon
        name="chevronDown"
        size="xs"
        class="text-gray-400 transition-transform duration-200"
        :class="{ 'rotate-180': isOpen }"
      />
    </button>

    <transition name="dropdown">
      <div
        v-if="isOpen"
        class="absolute right-0 z-50 overflow-hidden border shadow-lg"
        :class="props.variant === 'gateway'
          ? 'gateway-locale-menu mt-2 w-28 rounded-xl border-white/15 bg-slate-900/95 p-1.5 shadow-2xl backdrop-blur-2xl'
          : 'mt-1 w-32 rounded-lg border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-800'"
      >
        <button
          v-for="locale in availableLocales"
          :key="locale.code"
          :disabled="switching"
          @click="selectLocale(locale.code)"
          type="button"
          class="flex w-full items-center transition-colors"
          :class="props.variant === 'gateway'
            ? [
                'cursor-pointer justify-between rounded-lg px-2.5 py-1.5 text-xs',
                locale.code === currentLocaleCode
                  ? 'bg-cyan-500/20 text-cyan-200'
                  : 'text-white/70 hover:bg-white/10',
              ]
            : [
                'gap-2 px-3 py-2 text-sm text-gray-700 hover:bg-gray-100 dark:text-gray-200 dark:hover:bg-dark-700',
                locale.code === currentLocaleCode
                  ? 'bg-primary-50 text-primary-600 dark:bg-primary-900/20 dark:text-primary-400'
                  : '',
              ]"
        >
          <span v-if="props.variant !== 'gateway'" class="text-base">{{ locale.flag }}</span>
          <span>{{ props.variant === 'gateway' && locale.code === 'zh' ? '简体中文' : locale.name }}</span>
          <Icon
            v-if="locale.code === currentLocaleCode"
            name="check"
            :size="props.variant === 'gateway' ? 'xs' : 'sm'"
            :class="props.variant === 'gateway' ? 'text-cyan-300' : 'ml-auto text-primary-500'"
          />
        </button>
      </div>
    </transition>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onBeforeUnmount } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import { setLocale, availableLocales } from '@/i18n'

const { locale } = useI18n()

const props = withDefaults(defineProps<{
  variant?: 'default' | 'gateway'
}>(), {
  variant: 'default',
})

const isOpen = ref(false)
const dropdownRef = ref<HTMLElement | null>(null)
const switching = ref(false)

const currentLocaleCode = computed(() => locale.value)
const currentLocale = computed(() => availableLocales.find((l) => l.code === locale.value))
const gatewayLabel = computed(() => currentLocaleCode.value === 'zh' ? 'CN ZH' : 'EN US')

function toggleDropdown() {
  isOpen.value = !isOpen.value
}

async function selectLocale(code: string) {
  if (switching.value || code === currentLocaleCode.value) {
    isOpen.value = false
    return
  }
  switching.value = true
  try {
    await setLocale(code)
    isOpen.value = false
  } finally {
    switching.value = false
  }
}

function handleClickOutside(event: MouseEvent) {
  if (dropdownRef.value && !dropdownRef.value.contains(event.target as Node)) {
    isOpen.value = false
  }
}

onMounted(() => {
  document.addEventListener('click', handleClickOutside)
})

onBeforeUnmount(() => {
  document.removeEventListener('click', handleClickOutside)
})
</script>

<style scoped>
.dropdown-enter-active,
.dropdown-leave-active {
  transition: all 0.15s ease;
}

.gateway-locale-menu.dropdown-enter-active {
  transition: all 0.25s cubic-bezier(0.16, 1, 0.3, 1);
}

.dropdown-enter-from,
.dropdown-leave-to {
  opacity: 0;
  transform: scale(0.95) translateY(-4px);
}

.gateway-locale-menu.dropdown-enter-from {
  transform: scale(0.98);
}
</style>
