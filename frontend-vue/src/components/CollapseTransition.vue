<!-- CollapseTransition - 折叠展开动画组件 -->
<template>
  <transition name="collapse">
    <slot />
  </transition>
</template>

<script setup lang="ts">
</script>

<style scoped>
.collapse-enter-active,
.collapse-leave-active {
  transition: all 0.3s ease;
  overflow: hidden;
}

.collapse-enter-from,
.collapse-leave-to {
  opacity: 0;
  transform: scaleY(0);
}

.collapse-enter-to,
.collapse-leave-from {
  opacity: 1;
  transform: scaleY(1);
}
</style>
