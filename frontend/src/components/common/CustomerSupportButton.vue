<template>
  <template v-if="isEnabled">
    <button
      class="fixed bottom-5 right-5 z-[110] flex h-12 w-12 items-center justify-center rounded-full bg-primary-600 text-white shadow-lg transition hover:bg-primary-700 focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2 dark:focus:ring-offset-dark-900"
      type="button"
      :aria-label="t('common.contactSupport')"
      :title="t('common.contactSupport')"
      data-testid="customer-support-button"
      @click="open"
    >
      <Icon name="chatBubble" size="md" aria-hidden="true" />
    </button>
    <CustomerSupportModal v-model:visible="visible" />
  </template>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import CustomerSupportModal from './CustomerSupportModal.vue'
import { useAppStore } from '@/stores'

const { t } = useI18n()
const appStore = useAppStore()
const visible = ref(false)

const isEnabled = computed(() => (
  appStore.publicSettingsLoaded
  && appStore.cachedPublicSettings?.customer_support_enabled === true
  && (appStore.cachedPublicSettings?.customer_support_content || '').trim().length > 0
))

function open() {
  if (isEnabled.value) visible.value = true
}

watch(isEnabled, (enabled) => {
  if (!enabled) visible.value = false
})
</script>
