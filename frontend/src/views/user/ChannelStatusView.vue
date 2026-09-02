<template>
  <ChannelStatusV3View v-if="isV3 && !isV2Fallback" />
  <ChannelStatusV1View v-else-if="isV1" />
  <ChannelStatusV2View v-else />
</template>

<script setup lang="ts">
import { computed, defineAsyncComponent } from 'vue'
import { useRoute } from 'vue-router'
import { isChannelMonitorV1Mode, isChannelMonitorV3Mode } from '@/utils/featureFlags'
import ChannelStatusV1View from './ChannelStatusV1View.vue'
import ChannelStatusV2View from './ChannelStatusV2View.vue'

const isV1 = computed(() => isChannelMonitorV1Mode())
const isV3 = computed(() => isChannelMonitorV3Mode())
const route = useRoute()
const isV2Fallback = computed(() => isV3.value && route.query.monitor_view === 'v2')
const ChannelStatusV3View = defineAsyncComponent(() => import('./ChannelStatusV3View.vue'))
</script>
