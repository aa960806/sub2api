<template>
  <AppLayout>
    <div class="mx-auto max-w-6xl" data-testid="admin-battle-pass-page">
      <BattlePassConfig />
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import AppLayout from '@/components/layout/AppLayout.vue'
import BattlePassConfig from '@/components/admin/activity/BattlePassConfig.vue'
</script>
