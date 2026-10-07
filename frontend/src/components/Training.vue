<script setup lang="ts">
import { computed, onBeforeUnmount, ref } from "vue";
import type { AnswerResult, Training } from "../lib/api";
import { api, plural } from "../lib/api";
import { useKeydown } from "../lib/keys";
import Explanation from "./Explanation.vue";
import QuestionCard from "./QuestionCard.vue";

const props = defineProps<{
  training: Training;
  mode: "training" | "mistakes";
  title: string;
  resolveAfter: number;
  exitLabel?: string; // куда ведёт выход: по умолчанию «В меню»
}>();
const emit = defineEmits<{ exit: [] }>();

const total = props.training.questions.length;

const idx = ref(props.training.resume_index);
const picked = ref(0);
const res = ref<AnswerResult | null>(null);
const busy = ref(false);
const error = ref("");
const stats = ref({ answered: 0, correct: 0 });
const done = ref(false);
const showExpl = ref(false); // пояснение к верному ответу — по кнопке
let autoTimer: ReturnType<typeof setTimeout> | null = null;

const q = computed(() => props.training.questions[idx.value]);

onBeforeUnmount(() => clearAuto());

function clearAuto() {
  if (autoTimer) { clearTimeout(autoTimer); autoTimer = null; }
}

// позиция общая для всех устройств; не сохранилась — не страшно
function savePosition(i: number) {
  api.saveTrainingPosition(props.training.key, i).catch(() => {});
}

async function pick(n: number) {
  if (res.value || busy.value) return;
  busy.value = true;
  picked.value = n;
  try {
    const r = await api.recordAnswer(q.value.id, n, props.mode);
    res.value = r;
    stats.value = { answered: stats.value.answered + 1, correct: stats.value.correct + (r.correct ? 1 : 0) };
    // верный ответ — сам едет дальше, неверный ждёт решения человека
    if (r.correct) autoTimer = setTimeout(() => next(), 1000);
  } catch (e) {
    error.value = e instanceof Error ? e.message : String(e);
    picked.value = 0;
  } finally {
    busy.value = false;
  }
}

// «Пояснение» после верного ответа: отменяет автопереход
function explain() {
  clearAuto();
  showExpl.value = true;
}

function reset() {
  clearAuto();
  showExpl.value = false;
  picked.value = 0;
  res.value = null;
  error.value = "";
}

function next() {
  clearAuto();
  if (idx.value + 1 >= total) {
    savePosition(0);
    done.value = true;
    return;
  }
  idx.value += 1;
  reset();
  savePosition(idx.value);
}

function prev() {
  if (idx.value === 0) return;
  idx.value -= 1;
  reset();
  savePosition(idx.value);
}

function restart() {
  done.value = false;
  idx.value = 0;
  stats.value = { answered: 0, correct: 0 };
  reset();
  savePosition(0);
}

useKeydown((e) => {
  if (done.value) return;
  if (e.key >= "1" && e.key <= "9") {
    const n = Number(e.key);
    if (n <= q.value.options.length) void pick(n);
  } else if (e.key === "ArrowRight") {
    next();
  } else if (e.key === "Enter") {
    if (res.value) next();
  } else if (e.key === "ArrowLeft") {
    prev();
  }
});
</script>

<template>
  <div class="bar card">
    <strong class="title">{{ title }}</strong>
    <span class="spacer"></span>
    <span class="pill ok">верно {{ stats.correct }}</span>
    <span class="pill bad">ошибок {{ stats.answered - stats.correct }}</span>
    <button class="ghost" @click="emit('exit')">Выйти</button>
  </div>

  <section v-if="done" class="card finish">
    <h1>Готово</h1>
    <p class="dim">
      Пройдено {{ stats.answered }} {{ plural(stats.answered, "вопрос", "вопроса", "вопросов") }} из {{ total }},
      верно {{ stats.correct }}, с ошибкой {{ stats.answered - stats.correct }}.
      <template v-if="mode === 'mistakes'">
        Вопросы, на которые вы ответили верно {{ resolveAfter }} {{ plural(resolveAfter, "раз", "раза", "раз") }} подряд, ушли из списка.
      </template>
    </p>
    <div class="row">
      <button class="primary" @click="restart">Пройти заново</button>
      <button @click="emit('exit')">{{ exitLabel ?? "В меню" }}</button>
    </div>
  </section>

  <template v-else>
    <div class="progress"><div class="fill" :style="{ width: `${((idx + 1) / total) * 100}%` }"></div></div>

    <div class="row counter">
      <span class="dim">Вопрос {{ idx + 1 }} из {{ total }}</span>
      <span class="spacer"></span>
      <span class="dim keys"><kbd>1</kbd>…<kbd>5</kbd> ответ · <kbd>→</kbd> дальше</span>
    </div>

    <QuestionCard
      :q="q"
      :chosen="picked"
      :reveal="!!res"
      :answer="res?.answer ?? 0"
      :disabled="!!res || busy"
      @pick="pick"
    />

    <div class="row nav">
      <button :disabled="idx === 0" @click="prev">Назад</button>
      <template v-if="res">
        <button class="primary" @click="next">Далее</button>
        <button v-if="res.correct && !showExpl" @click="explain">Пояснение</button>
        <span v-if="res.correct" class="hint ok-text">Верно{{ showExpl ? "" : " — едем дальше" }}</span>
        <span v-else class="hint bad-text">
          Неверно.
          <template v-if="res.mistake.just_solved">Вопрос всё ещё в списке ошибок.</template>
          <template v-else-if="res.mistake.wrong_count > 1">Ошибка в этом вопросе уже {{ res.mistake.wrong_count }}-й раз.</template>
          <template v-else>Вопрос добавлен в список на проработку.</template>
        </span>
      </template>
      <button v-else @click="next">Пропустить</button>
      <span class="spacer"></span>
      <span v-if="res?.correct && res.mistake.wrong_count > 0" class="hint dim">
        <template v-if="res.mistake.just_solved">Проработан — убран из списка ошибок</template>
        <template v-else>Верных подряд: {{ res.mistake.streak }} из {{ resolveAfter }}</template>
      </span>
    </div>

    <Explanation
      v-if="res && (!res.correct || showExpl)"
      class="expl"
      :question-id="q.id"
      :answer="res.answer"
      :chosen="picked"
    />

    <p v-if="error" class="err">Ответ не сохранён: {{ error }}</p>
  </template>
</template>

<style scoped>
.bar { display: flex; align-items: center; gap: 8px; padding: 10px 14px; margin-bottom: 12px; }

.progress {
  height: 4px;
  background: var(--surface-2);
  border-radius: 999px;
  overflow: hidden;
  margin-bottom: 10px;
}
.fill { height: 100%; background: var(--accent); transition: width .2s; }

.counter { font-size: 13px; margin-bottom: 10px; }
.keys { font-size: 12px; }

.nav { gap: 10px; margin-top: 12px; flex-wrap: wrap; }
.hint { font-size: 13px; }
.ok-text { color: var(--ok); }
.bad-text { color: var(--bad); }
.err { color: var(--bad); font-size: 13.5px; }
.expl { margin-top: 14px; }

.finish { padding: 18px; display: grid; gap: 12px; }
.finish p { margin: 0; font-size: 14px; }

@media (max-width: 600px) {
  .bar { flex-wrap: wrap; padding: 8px 12px; }
  .title { flex-basis: 100%; font-size: 14px; }
}
</style>
