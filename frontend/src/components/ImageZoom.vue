<script setup lang="ts">
import { useKeydown } from "../lib/keys";

defineProps<{ src: string }>();
const emit = defineEmits<{ close: [] }>();

useKeydown((e) => {
  if (e.key === "Escape") { e.stopPropagation(); emit("close"); }
});
</script>

<template>
  <!-- оверлей: клик или тап в любом месте закрывает -->
  <div class="overlay" role="presentation" @click="emit('close')">
    <img :src="src" alt="Изображение к вопросу, увеличено" />
    <div class="hint">
      <span class="keys">Клик или <kbd>Esc</kbd> — закрыть</span>
      <span class="touch">Нажмите, чтобы закрыть</span>
    </div>
  </div>
</template>

<style scoped>
.overlay {
  position: fixed;
  inset: 0;
  z-index: 50;
  background: rgba(8, 11, 16, .82);
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 14px;
  padding: 24px 8px;
  cursor: zoom-out;
}
img {
  max-width: 100%;
  max-height: calc(100dvh - 100px);
  border-radius: 8px;
  background: #fff;
}
.hint { color: #c6cfdb; font-size: 13px; }
.touch { display: none; }
@media (hover: none) {
  .touch { display: inline; }
}
</style>
