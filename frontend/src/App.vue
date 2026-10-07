<script setup lang="ts">
import { ref } from "vue";
import type { ExamResult, ExamSession, MistakeRow, Selection, State, Stats, Training } from "./lib/api";
import { ApiError, SECTION_SHORT, api } from "./lib/api";
import { cycleTheme, themeLabel } from "./lib/theme";
import Home from "./components/Home.vue";
import ExamScreen from "./components/Exam.vue";
import ExamReview from "./components/ExamReview.vue";
import TrainingScreen from "./components/Training.vue";
import Mistakes from "./components/Mistakes.vue";
import StatsScreen from "./components/Stats.vue";
import Preview from "./components/Preview.vue";

type Screen = "loading" | "home" | "exam" | "review" | "training" | "mistakes" | "stats" | "preview";

const screen = ref<Screen>("loading");
const appState = ref<State | null>(null);
const session = ref<ExamSession | null>(null);
const result = ref<ExamResult | null>(null);
const training = ref<Training | null>(null);
const trainingMode = ref<"training" | "mistakes">("training");
const trainingTitle = ref("Тренировка");
// одиночный вопрос из списка ошибок возвращает обратно в список
const trainingBack = ref<"home" | "mistakes">("home");
const mistakes = ref<MistakeRow[]>([]);
const stats = ref<Stats | null>(null);
const error = ref("");
const busy = ref(false);
const askNew = ref(false);
const homeKey = ref(0); // пересоздать главную, чтобы она подхватила askNew

// guard оборачивает вызовы API: ошибку показываем, а не теряем молча
async function guard<T>(fn: () => Promise<T>): Promise<T | undefined> {
  try {
    error.value = "";
    return await fn();
  } catch (e) {
    error.value = e instanceof Error ? e.message : String(e);
    return undefined;
  }
}

async function loadState() {
  const s = await guard(() => api.getState());
  if (s) appState.value = s;
  return s;
}

// #q=<id вопроса> — просмотр вопроса с пояснением без записи ответа
const previewId = ref("");
function readHash() {
  const m = location.hash.match(/^#q=([\w-]+)$/);
  previewId.value = m ? m[1] : "";
  if (previewId.value) screen.value = "preview";
  else if (screen.value === "preview") void goHome();
}
window.addEventListener("hashchange", readHash);

async function init() {
  const s = await loadState();
  screen.value = s ? "home" : "loading";
  if (s) readHash();
}
void init();

async function goHome() {
  if (location.hash) history.replaceState(null, "", location.pathname);
  askNew.value = false;
  await loadState();
  homeKey.value++;
  screen.value = "home";
}

function openExam(s: ExamSession) {
  session.value = s;
  screen.value = "exam";
}

function showResult(r: ExamResult) {
  result.value = r;
  screen.value = "review";
  void loadState();
}

async function startExam() {
  busy.value = true;
  try {
    error.value = "";
    openExam(await api.startExam());
  } catch (e) {
    if (e instanceof ApiError && e.code === "exam_active") {
      // экзамен успели начать на другом устройстве — спрашиваем, что с ним делать
      await goHome();
      askNew.value = true;
      homeKey.value++;
      return;
    }
    error.value = e instanceof Error ? e.message : String(e);
  } finally {
    busy.value = false;
  }
}

async function resumeExam(id: number) {
  busy.value = true;
  try {
    error.value = "";
    const open = await api.resumeExam(id);
    if (open.result) showResult(open.result);
    else if (open.session) openExam(open.session);
  } catch (e) {
    error.value = e instanceof Error ? e.message : String(e);
    if (e instanceof ApiError && e.code === "no_exam") await goHome();
  } finally {
    busy.value = false;
  }
}

async function restartExam(id: number) {
  busy.value = true;
  try {
    await api.abandonExam(id);
  } catch (e) {
    if (!(e instanceof ApiError && e.code === "no_exam")) {
      error.value = e instanceof Error ? e.message : String(e);
      busy.value = false;
      return;
    }
  }
  busy.value = false;
  await startExam();
}

function selectionTitle(sel: Selection): string {
  switch (sel.scope) {
    case "group": {
      const g = appState.value?.groups.find((x) => x.group === sel.group);
      return g ? `Группа ${g.group} — ${g.title}` : `Группа ${sel.group}`;
    }
    case "section":
      return `Раздел: ${SECTION_SHORT[sel.section as keyof typeof SECTION_SHORT]}`;
    case "ids":
      return "Работа над ошибками";
    default:
      return "Вся категория";
  }
}

async function startTraining(sel: Selection, back: "home" | "mistakes" = "home") {
  const t = await guard(() => api.startTraining(sel));
  if (!t || t.questions.length === 0) {
    if (t) error.value = "В этой выборке нет вопросов";
    return;
  }
  training.value = t;
  trainingMode.value = sel.scope === "ids" ? "mistakes" : "training";
  trainingTitle.value = selectionTitle(sel);
  trainingBack.value = back;
  screen.value = "training";
}

async function practiceOne(id: string) {
  await startTraining({ scope: "ids", ids: [id] }, "mistakes");
}

async function leaveTraining() {
  if (trainingBack.value === "mistakes") await openMistakes();
  else await goHome();
}

async function openMistakes() {
  const rows = await guard(() => api.getMistakes());
  if (!rows) return;
  mistakes.value = rows;
  screen.value = "mistakes";
}

async function practiceMistakes() {
  await startTraining({ scope: "ids", ids: mistakes.value.map((m) => m.question.id) });
}

async function openStats() {
  const s = await guard(() => api.getStats());
  if (!s) return;
  stats.value = s;
  screen.value = "stats";
}
</script>

<template>
  <div class="app">
    <header class="topbar">
      <strong class="brand">Тренажёр теории ПДД</strong>
      <span class="pill">{{ appState?.meta.category ?? "ABC" }}</span>
      <span v-if="appState" class="pill wide-only">{{ appState.meta.total }} вопросов</span>
      <span class="spacer"></span>
      <span v-if="appState && appState.mistake_count > 0 && screen === 'home'" class="pill bad wide-only">
        ошибок {{ appState.mistake_count }}
      </span>
      <!-- из экзамена выходят только кнопкой «Выйти»: она бросает экзамен -->
      <button v-if="screen !== 'home' && screen !== 'loading' && screen !== 'exam'" class="ghost" @click="goHome">
        В меню
      </button>
      <button class="ghost" title="Переключить тему" @click="cycleTheme">{{ themeLabel }}</button>
    </header>

    <main class="content scroll-fade">
      <div class="content-inner">
        <div v-if="error" class="error card">
          <span>{{ error }}</span>
          <span class="spacer"></span>
          <button v-if="screen === 'loading'" class="ghost" @click="init">Повторить</button>
          <button v-else class="ghost" @click="error = ''">Скрыть</button>
        </div>

        <p v-if="screen === 'loading' && !error" class="dim">Загрузка банка вопросов…</p>
        <Home
          v-else-if="screen === 'home' && appState"
          :key="homeKey"
          :app-state="appState"
          :ask-new="askNew"
          :busy="busy"
          @exam="startExam"
          @resume="resumeExam"
          @restart="restartExam"
          @training="startTraining"
          @mistakes="openMistakes"
          @stats="openStats"
        />
        <ExamScreen
          v-else-if="screen === 'exam' && session"
          :key="session.exam_id"
          :session="session"
          @finish="showResult"
          @exit="goHome"
          @home="goHome"
        />
        <ExamReview
          v-else-if="screen === 'review' && result"
          :result="result"
          @again="startExam"
          @home="goHome"
          @mistakes="openMistakes"
        />
        <TrainingScreen
          v-else-if="screen === 'training' && training && appState"
          :training="training"
          :mode="trainingMode"
          :title="trainingTitle"
          :resolve-after="appState.resolve_after"
          :exit-label="trainingBack === 'mistakes' ? 'К списку ошибок' : undefined"
          @exit="leaveTraining"
        />
        <Mistakes
          v-else-if="screen === 'mistakes' && appState"
          :rows="mistakes"
          :resolve-after="appState.resolve_after"
          @practice="practiceMistakes"
          @practice-one="practiceOne"
          @home="goHome"
        />
        <StatsScreen v-else-if="screen === 'stats' && stats" :stats="stats" @home="goHome" />
        <Preview v-else-if="screen === 'preview' && previewId" :question-id="previewId" @home="goHome" />
      </div>
    </main>
  </div>
</template>

<style scoped>
.brand { font-size: 15.5px; white-space: nowrap; }
.error {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 10px 14px;
  margin-bottom: 14px;
  border-left: 4px solid var(--bad);
  color: var(--bad);
  font-size: 13.5px;
}
@media (max-width: 600px) {
  .brand { font-size: 14.5px; }
}
</style>
