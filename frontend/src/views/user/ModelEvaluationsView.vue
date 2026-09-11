<template>
  <AppLayout>
    <div class="space-y-6">
      <header class="card user-glass-panel space-y-3 p-5 sm:p-6">
        <h1 class="page-title">{{ t('modelEvaluations.title') }}</h1>
        <p class="text-sm text-gray-600 dark:text-gray-300">{{ t('modelEvaluations.description') }}</p>
        <p class="rounded-xl bg-gray-100 p-3 text-sm dark:bg-dark-900"><span class="font-medium">{{ t('modelEvaluations.prompt') }}：</span>{{ MODEL_EVALUATION_PROMPT }}</p>
      </header>
      <p v-if="loading" role="status">{{ t('modelEvaluations.loading') }}</p>
      <div v-else-if="error" role="alert" class="card user-glass-panel p-6"><p>{{ t('modelEvaluations.loadFailed') }}</p><button type="button" class="btn btn-secondary mt-3" @click="load">{{ t('modelEvaluations.retry') }}</button></div>
      <p v-else-if="!enabled" class="card user-glass-panel p-6">{{ t('modelEvaluations.disabled') }}</p>
      <p v-else-if="!groups.length" class="card user-glass-panel p-6">{{ t('modelEvaluations.noGroups') }}</p>
      <ModelEvaluationGallery v-else :groups="groups" />
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import ModelEvaluationGallery from '@/components/model-evaluation/ModelEvaluationGallery.vue'
import { MODEL_EVALUATION_PROMPT, modelEvaluationsAPI, type ModelEvaluationGroup } from '@/api/modelEvaluations'

const { t } = useI18n()
const groups = ref<ModelEvaluationGroup[]>([])
const enabled = ref(false)
const loading = ref(true)
const error = ref(false)
const controller = new AbortController()
async function load() {
  loading.value = true
  error.value = false
  try {
    const result = await modelEvaluationsAPI.groups(controller.signal)
    if (controller.signal.aborted) return
    enabled.value = result.enabled
    groups.value = result.items
  } catch { if (!controller.signal.aborted) error.value = true }
  finally { loading.value = false }
}
onMounted(load)
onBeforeUnmount(() => controller.abort())
</script>
