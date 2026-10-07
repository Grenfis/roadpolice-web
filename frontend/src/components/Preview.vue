<script setup lang="ts">
// Просмотр вопроса с пояснением по ссылке #q=<id>. Ответ здесь не
// записывается — статистика и список ошибок не меняются.
import { ref, watch } from "vue";
import type { Question } from "../lib/api";
import { api } from "../lib/api";
import Explanation from "./Explanation.vue";
import QuestionCard from "./QuestionCard.vue";

const props = defineProps<{ questionId: string }>();
const emit = defineEmits<{ home: [] }>();

const q = ref<Question | null>(null);
const error = ref("");

watch(() => props.questionId, async (id) => {
  q.value = null;
  error.value = "";
  try {
    // выборка «по списку id» ничего не пишет в историю ответов
    const t = await api.startTraining({ scope: "ids", ids: [id] });
    if (t.questions.length === 0) error.value = `Вопроса ${id} нет в банке`;
    else q.value = t.questions[0];
  } catch (e) {
    error.value = e instanceof Error ? e.message : String(e);
  }
}, { immediate: true });
</script>

<template>
  <div class="bar card">
    <strong>Просмотр вопроса</strong>
    <span class="dim wide-only">· ответ не записывается в статистику</span>
    <span class="spacer"></span>
    <button class="ghost" @click="emit('home')">В меню</button>
  </div>
  <p v-if="error" class="err">{{ error }}</p>
  <div v-else-if="q" class="wrap">
    <QuestionCard :q="q" :reveal="true" :answer="q.answer" :disabled="true" />
    <Explanation :question-id="q.id" :answer="q.answer" :chosen="0" :expanded="true" />
  </div>
</template>

<style scoped>
.bar { display: flex; align-items: center; gap: 8px; padding: 10px 14px; margin-bottom: 12px; }
.wrap { display: grid; gap: 12px; }
.err { color: var(--bad); }
</style>
