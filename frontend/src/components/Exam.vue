<script setup lang="ts">
// Экзамен живёт на сервере: каждый выбор сразу сохраняется, таймер считает
// сервер, а здесь он только тикает между синхронизациями. Закрытие страницы
// ставит таймер на паузу (sendBeacon на pagehide); свёрнутая вкладка — нет.
// Если экзамен открыли на другом устройстве, этот экран узнаёт об этом при
// следующем запросе (ответ или опрос раз в 10 секунд) и блокируется.
import { computed, onBeforeUnmount, onMounted, ref, watch } from "vue";
import type { ExamResult, ExamSession } from "../lib/api";
import { ApiError, api, fmtTime } from "../lib/api";
import { useKeydown } from "../lib/keys";
import QuestionCard from "./QuestionCard.vue";

const props = defineProps<{ session: ExamSession }>();
const emit = defineEmits<{ finish: [r: ExamResult]; exit: []; home: [] }>();

const examID = props.session.exam_id;
const questions = props.session.questions;
const total = questions.length;
const POLL_MS = 10_000;

const lease = ref(props.session.lease);
const chosen = ref<Record<string, number>>({ ...props.session.chosen });
// открываем на первом неотвеченном вопросе
const idx = ref(Math.max(0, questions.findIndex((q) => chosen.value[q.id] === undefined)));

const deadline = ref(0);
const now = ref(performance.now());
function sync(remainingMs: number) {
  now.value = performance.now();
  deadline.value = now.value + remainingMs;
}
sync(props.session.remaining_ms);
const left = computed(() => Math.max(0, Math.ceil((deadline.value - now.value) / 1000)));

const finishing = ref(false);
const done = ref(false); // сдан или брошен — больше ничего не шлём
const confirmFinish = ref(false);
const confirmExit = ref(false);
const lost = ref<"" | "lease" | "gone">("");
const error = ref("");
const reclaiming = ref(false);

const q = computed(() => questions[idx.value]);
const answeredCount = computed(() => Object.keys(chosen.value).length);
const locked = computed(() => finishing.value || done.value || lost.value !== "");

// ошибки экзамена: потеря экзамена блокирует экран, остальное — текстом
function handle(e: unknown, prefix: string) {
  if (e instanceof ApiError && e.code === "lease_lost") lost.value = "lease";
  else if (e instanceof ApiError && e.code === "no_exam") lost.value = "gone";
  else error.value = `${prefix}: ${e instanceof Error ? e.message : String(e)}`;
}

// ответы уходят строго по очереди, чтобы быстрый перещёлк не перепутал порядок
let saving: Promise<void> = Promise.resolve();

function pick(n: number) {
  if (locked.value || left.value === 0) return;
  const id = q.value.id;
  const prev = chosen.value[id];
  chosen.value = { ...chosen.value, [id]: n };
  error.value = "";
  saving = saving.then(async () => {
    try {
      const tick = await api.answerExam(examID, lease.value, id, n);
      sync(tick.remaining_ms);
    } catch (e) {
      handle(e, "Ответ не сохранён");
      if (!lost.value && chosen.value[id] === n) {
        const next = { ...chosen.value };
        if (prev === undefined) delete next[id];
        else next[id] = prev;
        chosen.value = next;
      }
    }
  });
}

function go(to: number) {
  confirmFinish.value = false;
  idx.value = Math.min(total - 1, Math.max(0, to));
}

async function finish() {
  if (finishing.value || done.value || lost.value) return;
  finishing.value = true;
  error.value = "";
  try {
    await saving;
    if (lost.value) return;
    const res = await api.finishExam(examID, lease.value);
    done.value = true;
    emit("finish", res);
  } catch (e) {
    handle(e, "Не удалось завершить экзамен");
  } finally {
    finishing.value = false;
  }
}

async function abandon() {
  if (finishing.value) return;
  finishing.value = true;
  try {
    await api.abandonExam(examID);
    done.value = true;
    emit("exit");
  } catch (e) {
    if (e instanceof ApiError && e.code === "no_exam") {
      done.value = true;
      emit("exit");
      return;
    }
    handle(e, "Не удалось бросить экзамен");
  } finally {
    finishing.value = false;
  }
}

// «Продолжить здесь» после перехвата: забираем экзамен обратно
async function reclaim() {
  reclaiming.value = true;
  try {
    const open = await api.resumeExam(examID);
    if (open.result) {
      done.value = true;
      emit("finish", open.result);
      return;
    }
    if (open.session) {
      lease.value = open.session.lease;
      chosen.value = { ...open.session.chosen };
      sync(open.session.remaining_ms);
      lost.value = "";
      error.value = "";
    }
  } catch (e) {
    handle(e, "Не удалось продолжить");
  } finally {
    reclaiming.value = false;
  }
}

async function poll() {
  if (locked.value) return;
  try {
    sync((await api.examStatus(examID, lease.value)).remaining_ms);
  } catch (e) {
    // нет связи — молчим, таймер тикает по локальным часам
    if (e instanceof ApiError && e.code !== "network") handle(e, "Связь с сервером");
  }
}

// время вышло — сдаём сами, с теми ответами, что успели
watch(left, (v) => { if (v === 0) void finish(); });

function onPageHide() {
  if (!done.value && !lost.value) api.pauseExam(examID, lease.value);
}
// страницу вернули из bfcache после pagehide: на сервере таймер на паузе,
// открываем экзамен заново
function onPageShow(e: PageTransitionEvent) {
  if (e.persisted && !done.value) void reclaim();
}
function onVisible() {
  if (document.visibilityState === "visible") void poll();
}

let ticker = 0;
let poller = 0;
onMounted(() => {
  ticker = window.setInterval(() => (now.value = performance.now()), 250);
  poller = window.setInterval(() => void poll(), POLL_MS);
  window.addEventListener("pagehide", onPageHide);
  window.addEventListener("pageshow", onPageShow);
  document.addEventListener("visibilitychange", onVisible);
});
onBeforeUnmount(() => {
  clearInterval(ticker);
  clearInterval(poller);
  window.removeEventListener("pagehide", onPageHide);
  window.removeEventListener("pageshow", onPageShow);
  document.removeEventListener("visibilitychange", onVisible);
});

useKeydown((e) => {
  if (locked.value) return;
  if (e.key >= "1" && e.key <= "9") {
    const n = Number(e.key);
    if (n <= q.value.options.length) pick(n);
  } else if (e.key === "ArrowRight" || e.key === "Enter") {
    go(idx.value + 1);
  } else if (e.key === "ArrowLeft") {
    go(idx.value - 1);
  }
});
</script>

<template>
  <section v-if="lost" class="card lost">
    <template v-if="lost === 'lease'">
      <h2>Экзамен продолжен на другом устройстве</h2>
      <p class="dim">Здесь отвечать больше нельзя. Можно забрать экзамен обратно на это устройство.</p>
      <div class="row">
        <button class="primary" :disabled="reclaiming" @click="reclaim">Продолжить здесь</button>
        <button @click="emit('home')">В меню</button>
      </div>
    </template>
    <template v-else>
      <h2>Экзамена больше нет</h2>
      <p class="dim">Его уже сдали или бросили на другом устройстве. Результат — в статистике.</p>
      <div class="row">
        <button class="primary" @click="emit('home')">В меню</button>
      </div>
    </template>
    <p v-if="error" class="err">{{ error }}</p>
  </section>

  <template v-else>
    <div class="bar card">
      <strong class="clock mono">{{ fmtTime(left) }}</strong>
      <span class="dim wide-only">осталось</span>
      <span class="spacer"></span>
      <span class="dim">{{ answeredCount }}/{{ total }}<span class="wide-only"> отвечено</span></span>
      <button class="ghost" :disabled="finishing" @click="confirmExit = !confirmExit">Выйти</button>
    </div>

    <div v-if="confirmExit" class="card confirm">
      <span>Бросить экзамен? Он не попадёт ни в историю, ни в список ошибок.</span>
      <div class="row">
        <button @click="confirmExit = false">Отмена</button>
        <button class="danger" :disabled="finishing" @click="abandon">Бросить</button>
      </div>
    </div>

    <div class="pads">
      <button
        v-for="(item, i) in questions"
        :key="item.id"
        :class="['pad', { current: i === idx, done: chosen[item.id] !== undefined }]"
        :title="`Вопрос ${i + 1}`"
        @click="go(i)"
      >{{ i + 1 }}</button>
    </div>

    <div class="qwrap">
      <div class="row counter">
        <span class="dim">Вопрос {{ idx + 1 }} из {{ total }}</span>
        <span class="spacer"></span>
        <span class="dim keys"><kbd>1</kbd>…<kbd>5</kbd> ответ · <kbd>←</kbd><kbd>→</kbd> переход</span>
      </div>

      <QuestionCard :q="q" :chosen="chosen[q.id] ?? 0" :show-meta="false" :disabled="locked" @pick="pick" />

      <div class="row nav">
        <button :disabled="idx === 0" @click="go(idx - 1)">Назад</button>
        <button :disabled="idx === total - 1" @click="go(idx + 1)">Далее</button>
        <span class="spacer"></span>
        <template v-if="confirmFinish">
          <span class="dim warn">
            <template v-if="answeredCount < total">Без ответа {{ total - answeredCount }} — они пойдут в минус.</template>
            <template v-else>Завершить и посмотреть результат?</template>
          </span>
          <button @click="confirmFinish = false">Отмена</button>
          <button class="primary" :disabled="finishing" @click="finish">Завершить</button>
        </template>
        <button v-else class="primary" :disabled="finishing" @click="confirmFinish = true">
          Завершить экзамен
        </button>
      </div>

      <p v-if="error" class="err">{{ error }}</p>
    </div>
  </template>
</template>

<style scoped>
.bar { display: flex; align-items: center; gap: 10px; padding: 10px 14px; margin-bottom: 12px; }
.clock { font-size: 20px; }

.confirm {
  display: grid;
  gap: 10px;
  padding: 12px 14px;
  margin-bottom: 12px;
  border-left: 4px solid var(--bad);
  font-size: 14px;
}
.confirm .row { justify-content: flex-end; }
.danger { background: var(--bad); border-color: var(--bad); color: var(--surface); font-weight: 600; }
.danger:hover:not(:disabled) { background: var(--bad); filter: brightness(1.08); }

.pads { display: flex; flex-wrap: wrap; gap: 6px; margin-bottom: 14px; }
.pad {
  width: 34px;
  padding: 5px 0;
  text-align: center;
  font-size: 13px;
  background: var(--surface);
}
.pad.done { background: var(--surface-2); border-color: var(--text-dim); font-weight: 650; }
.pad.current { border-color: var(--accent); box-shadow: inset 0 0 0 1px var(--accent); }

.qwrap { display: grid; gap: 12px; }
.counter { font-size: 13px; }
.keys { font-size: 12px; }
.nav { gap: 10px; flex-wrap: wrap; }
.warn { color: var(--warn); font-size: 13px; }
.err { color: var(--bad); font-size: 13.5px; margin: 0; }

.lost { padding: 18px; display: grid; gap: 12px; border-left: 4px solid var(--warn); }
.lost p { margin: 0; font-size: 14px; }

/* на узком экране — 10 номеров в ряд на всю ширину */
@media (max-width: 600px) {
  .pads { display: grid; grid-template-columns: repeat(10, 1fr); gap: 5px; }
  .pad { width: auto; min-height: 36px; padding: 0; }
  .bar { padding: 8px 12px; }
  .pad.done:hover:not(:disabled) { background: var(--surface-2); }
}
</style>
