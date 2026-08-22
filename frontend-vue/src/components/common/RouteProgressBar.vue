<script setup lang="ts">
import { computed } from 'vue'
import { routeLoading } from '../../router/progress'

const barStyle = computed(() => ({
  opacity: routeLoading.value ? '1' : '0',
  transform: routeLoading.value ? 'translateX(0)' : 'translateX(-100%)',
}))
</script>

<template>
  <div class="route-progress" :style="barStyle" aria-hidden="true" />
</template>

<style scoped>
.route-progress {
  position: fixed;
  top: 0; left: 0;
  height: 2px;
  width: 100%;
  background: var(--primary, #4176e6);
  z-index: 9999;
  pointer-events: none;
  transition: opacity 0.2s ease, transform 0.3s ease;
  box-shadow: 0 0 8px var(--primary, #4176e6);
}
</style>
