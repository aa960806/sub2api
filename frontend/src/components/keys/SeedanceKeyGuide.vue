<template>
  <div class="space-y-4" data-testid="seedance-key-guide">
    <p class="text-sm text-gray-600 dark:text-gray-400">{{ t('keys.useKeyModal.seedance.description') }}</p>
    <p class="text-sm text-amber-700 dark:text-amber-300">{{ t('keys.useKeyModal.seedance.note') }}</p>
    <div v-for="sample in samples" :key="sample.title" class="space-y-2">
      <p class="text-sm font-medium text-gray-900 dark:text-white">{{ sample.title }}</p>
      <pre class="overflow-x-auto rounded-xl bg-gray-900 p-4 text-xs text-gray-100"><code>{{ sample.content }}</code></pre>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'

const props = defineProps<{ baseUrl: string; apiKey: string }>()
const { t } = useI18n()
const mediaBase = computed(() => `${props.baseUrl.replace(/\/+$/, '').replace(/\/v1$/, '')}/v1/media`)
const samples = computed(() => [
  {
    title: t('keys.useKeyModal.seedance.create'),
    content: `POST ${mediaBase.value}/videos
Authorization: Bearer ${props.apiKey}
Content-Type: application/json
Idempotency-Key: <unique-request-id>

${JSON.stringify({ model: 'seedance2.0mini', prompt: 'A paper boat floating on a quiet lake', duration: 5, ratio: '16:9', resolution: '720p', camera_movement: 'auto' }, null, 2)}`
  },
  {
    title: t('keys.useKeyModal.seedance.poll'),
    content: `GET ${mediaBase.value}/videos/<task_id>
Authorization: Bearer ${props.apiKey}`
  },
  {
    title: t('keys.useKeyModal.seedance.download'),
    content: `GET ${mediaBase.value}/videos/<task_id>/content
Authorization: Bearer ${props.apiKey}`
  }
])
</script>
