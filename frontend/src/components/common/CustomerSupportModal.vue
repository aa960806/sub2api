<template>
  <Teleport to="body">
    <Transition name="customer-support-fade">
      <div v-if="visible && hasContent" class="fixed inset-0 z-[120] flex items-center justify-center bg-black/55 p-4 backdrop-blur-sm" @click.self="close">
        <section class="flex max-h-[85vh] w-full max-w-xl flex-col overflow-hidden rounded-lg border border-gray-200 bg-white shadow-2xl dark:border-dark-700 dark:bg-dark-900" role="dialog" aria-modal="true" :aria-label="title">
          <header class="flex items-center justify-between gap-3 border-b border-gray-200 px-5 py-4 dark:border-dark-700">
            <div class="flex min-w-0 items-center gap-3">
              <span class="flex h-9 w-9 shrink-0 items-center justify-center rounded-md bg-primary-600 text-white"><Icon name="chatBubble" size="md" aria-hidden="true" /></span>
              <h2 class="truncate text-base font-semibold text-gray-900 dark:text-white">{{ title }}</h2>
            </div>
            <button class="btn-icon shrink-0" type="button" :aria-label="t('common.close')" :title="t('common.close')" @click="close"><Icon name="x" size="sm" aria-hidden="true" /></button>
          </header>
          <div class="markdown-body min-h-0 overflow-y-auto px-5 py-5" v-html="renderedContent"></div>
        </section>
      </div>
    </Transition>
  </Teleport>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { marked } from 'marked'
import DOMPurify from 'dompurify'
import Icon from '@/components/icons/Icon.vue'
import { useAppStore } from '@/stores'
import '@/styles/announcement-markdown.css'

const props = defineProps<{ visible: boolean }>()
const emit = defineEmits<{ (event: 'update:visible', value: boolean): void }>()
const { t } = useI18n()
const appStore = useAppStore()

const rawContent = computed(() => appStore.cachedPublicSettings?.customer_support_content || '')
const hasContent = computed(() => rawContent.value.trim().length > 0)
const title = computed(() => {
  const siteName = appStore.cachedPublicSettings?.site_name?.trim()
  return siteName ? `${siteName} · ${t('common.contactSupport')}` : t('common.contactSupport')
})

marked.setOptions({ breaks: true, gfm: true })

const renderedContent = computed(() => {
  if (!hasContent.value) return ''
  try {
    const html = marked.parse(rawContent.value) as string
    return DOMPurify.sanitize(html, {
      // Keep the support channel useful for QR images while stripping scripts
      // and event-handler attributes from administrator-authored Markdown.
      ADD_ATTR: ['target', 'rel'],
      ALLOW_DATA_ATTR: false,
    })
  } catch {
    return ''
  }
})

function close() {
  emit('update:visible', false)
}

function onKeydown(event: KeyboardEvent) {
  if (event.key === 'Escape' && props.visible) close()
}

watch(
  () => [props.visible, hasContent.value] as const,
  ([visible, content]) => {
    if (!visible || !content) {
      if (props.visible && !content) close()
      document.body.style.overflow = ''
    } else {
      document.body.style.overflow = 'hidden'
    }
  },
  { immediate: true },
)

onBeforeUnmount(() => {
  document.body.style.overflow = ''
})

onMounted(() => window.addEventListener('keydown', onKeydown))
onBeforeUnmount(() => window.removeEventListener('keydown', onKeydown))
</script>

<style scoped>
.customer-support-fade-enter-active,
.customer-support-fade-leave-active {
  transition: opacity 0.18s ease;
}

.customer-support-fade-enter-from,
.customer-support-fade-leave-to {
  opacity: 0;
}
</style>
