<template>
  <AppLayout>
    <div class="mx-auto max-w-6xl" data-testid="admin-student-recharge-page">
      <StudentRechargeBenefitConfig />
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import AppLayout from '@/components/layout/AppLayout.vue'
import StudentRechargeBenefitConfig from '@/components/admin/activity/StudentRechargeBenefitConfig.vue'
</script>
