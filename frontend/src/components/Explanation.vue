<script setup lang="ts">
// Пояснение к вопросу: почему верен правильный ответ и почему не подходит
// выбранный неверный; разбор остальных вариантов — по кнопке.
import { computed, ref, watch } from "vue";
import type { Explanation } from "../lib/api";
import { api } from "../lib/api";
import ExplanationPart from "./ExplanationPart.vue";

const props = defineProps<{
  questionId: string;
  answer: number; // номер верного варианта
  chosen: number; // выбранный; 0 — не отвечено
  expanded?: boolean; // сразу показать разбор всех вариантов
}>();

const data = ref<Explanation | null>(null);
const loaded = ref(false);
const error = ref("");
const showOthers = ref(!!props.expanded);

async function load() {
  try {
    data.value = await api.getExplanation(props.questionId);
    error.value = "";
  } catch (e) {
    error.value = e instanceof Error ? e.message : String(e);
  } finally {
    loaded.value = true;
  }
}
watch(() => props.questionId, () => {
  loaded.value = false;
  showOthers.value = !!props.expanded;
  void load();
}, { immediate: true });

const chosenWrong = computed(() =>
  props.chosen > 0 && props.chosen !== props.answer ? String(props.chosen) : "");
const others = computed(() =>
  Object.keys(data.value?.options ?? {})
    .filter((k) => k !== chosenWrong.value)
    .sort((a, b) => Number(a) - Number(b)),
);
</script>

<template>
  <div v-if="loaded" class="explanation">
    <p v-if="error" class="dim note">Пояснение не загрузилось: {{ error }}</p>
    <p v-else-if="!data" class="dim note">Пояснения к этому вопросу пока нет.</p>
    <template v-else>
      <p v-if="data.no_basis" class="note warn">
        Пояснение без опоры на нормативный текст: в правилах нет пунктов о первой помощи.
      </p>
      <ExplanationPart
        v-if="data.answer"
        :question-id="questionId"
        part-key="answer"
        :title="`Почему верно: вариант ${answer}`"
        kind="ok"
        :part="data.answer"
        @changed="load"
      />
      <ExplanationPart
        v-if="chosenWrong && data.options[chosenWrong]"
        :question-id="questionId"
        :part-key="chosenWrong"
        :title="`Почему не вариант ${chosenWrong}`"
        kind="bad"
        :part="data.options[chosenWrong]"
        @changed="load"
      />
      <template v-if="others.length">
        <button class="ghost toggle" @click="showOthers = !showOthers">
          {{ showOthers ? "Скрыть разбор остальных вариантов" : `Разбор остальных вариантов (${others.length})` }}
        </button>
        <template v-if="showOthers">
          <ExplanationPart
            v-for="k in others"
            :key="k"
            :question-id="questionId"
            :part-key="k"
            :title="`Почему не вариант ${k}`"
            kind="bad"
            :part="data.options[k]"
            @changed="load"
          />
        </template>
      </template>
    </template>
  </div>
</template>

<style scoped>
.explanation { display: grid; gap: 10px; }
.note { margin: 0; font-size: 13px; }
.warn { color: var(--warn); }
.toggle { justify-self: start; font-size: 13px; padding: 6px 10px; }
</style>
