<script setup lang="ts">
import { computed, ref } from "vue";
import type { ExamResult } from "../lib/api";
import { fmtTime, plural } from "../lib/api";
import Explanation from "./Explanation.vue";
import QuestionCard from "./QuestionCard.vue";

const props = defineProps<{ result: ExamResult }>();
const emit = defineEmits<{ again: []; home: []; mistakes: [] }>();

const wrong = computed(() => props.result.items.filter((i) => !i.correct).length);
const skipped = computed(() => props.result.items.filter((i) => i.chosen === 0).length);
const onlyWrong = ref(false);
const shown = computed(() =>
  props.result.items
    .map((item, i) => ({ item, num: i + 1 }))
    .filter(({ item }) => !onlyWrong.value || !item.correct),
);
</script>

<template>
  <section :class="['card', 'verdict', { passed: result.passed }]">
    <div class="row">
      <h1>{{ result.passed ? "Экзамен сдан" : "Экзамен не сдан" }}</h1>
      <span class="spacer"></span>
      <strong class="score mono">{{ result.correct }} / {{ result.total }}</strong>
    </div>
    <div class="row facts">
      <span class="pill">время {{ fmtTime(result.elapsed_sec) }}</span>
      <span :class="['pill', { bad: wrong > 0, ok: wrong === 0 }]">ошибок {{ wrong }}</span>
      <span v-if="skipped > 0" class="pill bad">без ответа {{ skipped }}</span>
      <span v-if="result.timed_out" class="pill bad">время вышло</span>
    </div>
    <p class="dim">
      <template v-if="result.passed">
        Порог — 18 из 20. {{ wrong === 0 ? "Ни одной ошибки." : `Запас: ${result.correct - 18} ${plural(result.correct - 18, "ответ", "ответа", "ответов")}.` }}
      </template>
      <template v-else>Нужно минимум 18 верных из 20. Все ошибки добавлены в список на проработку.</template>
    </p>
    <div class="row actions">
      <button class="primary" @click="emit('again')">Ещё один билет</button>
      <button :disabled="wrong === 0" @click="emit('mistakes')">К списку ошибок</button>
      <span class="spacer"></span>
      <button class="ghost" @click="emit('home')">В меню</button>
    </div>
  </section>

  <div class="row filter">
    <h2>Разбор билета</h2>
    <span class="spacer"></span>
    <label class="check">
      <input v-model="onlyWrong" type="checkbox" :disabled="wrong === 0" />
      <span>только ошибки</span>
    </label>
  </div>

  <div class="list">
    <div v-for="{ item, num } in shown" :key="item.question.id" class="item">
      <div class="row head">
        <span class="n">{{ num }}</span>
        <span v-if="item.correct" class="pill ok">верно</span>
        <span v-else-if="item.chosen === 0" class="pill bad">без ответа</span>
        <span v-else class="pill bad">ошибка</span>
      </div>
      <QuestionCard :q="item.question" :chosen="item.chosen" :reveal="true" :answer="item.question.answer" :disabled="true" />
      <Explanation :question-id="item.question.id" :answer="item.question.answer" :chosen="item.chosen" />
    </div>
  </div>
</template>

<style scoped>
.verdict {
  padding: 16px 18px;
  display: grid;
  gap: 10px;
  border-left: 4px solid var(--bad);
}
.verdict.passed { border-left-color: var(--ok); }
.verdict p { margin: 0; font-size: 13.5px; }
.score { font-size: 24px; font-weight: 700; }
.facts { gap: 8px; flex-wrap: wrap; }
.actions { flex-wrap: wrap; }

.filter { margin: 18px 0 10px; }
.check { display: flex; align-items: center; gap: 7px; cursor: pointer; font-size: 13.5px; }

.list { display: grid; gap: 14px; }
.item { display: grid; gap: 7px; }
.head { gap: 8px; }
.n {
  width: 26px; height: 26px;
  display: grid; place-items: center;
  border-radius: 7px;
  background: var(--surface-2);
  border: 1px solid var(--border);
  font-size: 13px; font-weight: 650;
}

@media (max-width: 600px) {
  .verdict { padding: 12px 14px; }
  h1 { font-size: 19px; }
}
</style>
