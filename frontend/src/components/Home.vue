<script setup lang="ts">
import { computed, ref } from "vue";
import type { Selection, State } from "../lib/api";
import { SECTION_SHORT, fmtTime, plural } from "../lib/api";

const props = defineProps<{
  appState: State;
  askNew: boolean; // сразу показать вопрос «продолжить или бросить»
  busy: boolean;
}>();
const emit = defineEmits<{
  exam: [];
  resume: [id: number];
  restart: [id: number]; // бросить незавершённый и начать новый
  training: [sel: Selection];
  mistakes: [];
  stats: [];
}>();

const scope = ref<"all" | "group" | "section">("all");
const group = ref(props.appState.groups[0]?.group ?? 1);
const section = ref<string>(props.appState.section_order[0] ?? "law");
const shuffle = ref(false);
const confirmNew = ref(props.askNew);

const active = computed(() => props.appState.active_exam);

const scopeCount = computed(() =>
  scope.value === "all"
    ? props.appState.meta.total
    : scope.value === "group"
      ? (props.appState.groups.find((g) => g.group === group.value)?.count ?? 0)
      : ((props.appState.by_section as Record<string, number>)[section.value] ?? 0),
);

function newExam() {
  if (active.value) confirmNew.value = true;
  else emit("exam");
}

function start() {
  emit("training", { scope: scope.value, group: group.value, section: section.value, shuffle: shuffle.value });
}
</script>

<template>
  <div class="grid">
    <section class="card block exam">
      <div class="row head">
        <h2>Экзамен</h2>
        <span class="spacer"></span>
        <span class="pill">{{ appState.meta.exam.questions }} вопросов</span>
        <span class="pill">{{ appState.meta.exam.time_minutes }} минут</span>
        <span class="pill">порог {{ appState.meta.exam.pass_min_correct }}</span>
      </div>
      <p class="dim">
        Билет собирается по квотам разделов, как на настоящем экзамене:
        <template v-for="(sid, i) in appState.section_order" :key="sid">{{ i > 0 ? ", " : "" }}{{ appState.meta.sections[sid].per_ticket }}
          × {{ SECTION_SHORT[sid] }}</template>.
        Подсветки нет, ответы — в конце.
      </p>

      <div v-if="active" class="active">
        <div>
          <strong>Незавершённый экзамен</strong>
          <span class="dim">
            · отвечено {{ active.answered }} из {{ active.total }}
            · {{ active.remaining_ms > 0 ? `осталось ${fmtTime(Math.ceil(active.remaining_ms / 1000))}` : "время вышло" }}
          </span>
        </div>
        <div v-if="confirmNew" class="ask">
          <span>Продолжить его или бросить и начать новый? Брошенный не попадёт ни в историю, ни в ошибки.</span>
          <div class="row actions">
            <button class="primary" :disabled="busy" @click="emit('resume', active.exam_id)">Продолжить</button>
            <button class="danger" :disabled="busy" @click="emit('restart', active.exam_id)">Бросить и начать новый</button>
            <button class="ghost" @click="confirmNew = false">Отмена</button>
          </div>
        </div>
        <div v-else class="row actions">
          <button class="primary big" :disabled="busy" @click="emit('resume', active.exam_id)">
            {{ active.remaining_ms > 0 ? "Продолжить экзамен" : "Посмотреть результат" }}
          </button>
          <button :disabled="busy" @click="newExam">Новый экзамен</button>
        </div>
      </div>
      <button v-else class="primary big" :disabled="busy" @click="newExam">Начать экзамен</button>
    </section>

    <section class="card block">
      <div class="row head">
        <h2>Тренировка</h2>
        <span class="spacer"></span>
        <span class="pill">без времени</span>
        <span class="pill">подсветка сразу</span>
      </div>
      <p class="dim">
        Верный ответ — переход к следующему вопросу через секунду.
        Неверный — показывается правильный, дальше вручную.
      </p>

      <div class="choices">
        <label class="choice">
          <input v-model="scope" type="radio" value="all" />
          <span>Вся категория</span>
          <span class="dim count">{{ appState.meta.total }}</span>
        </label>

        <label class="choice">
          <input v-model="scope" type="radio" value="group" />
          <span>Группа</span>
          <select v-model="group" @click="scope = 'group'" @change="scope = 'group'">
            <option v-for="g in appState.groups" :key="g.group" :value="g.group">
              {{ g.group }} — {{ g.title }} ({{ g.count }})
            </option>
          </select>
        </label>

        <label class="choice">
          <input v-model="scope" type="radio" value="section" />
          <span>Раздел</span>
          <select v-model="section" @click="scope = 'section'" @change="scope = 'section'">
            <option v-for="sid in appState.section_order" :key="sid" :value="sid">
              {{ SECTION_SHORT[sid] }} ({{ appState.by_section[sid] }})
            </option>
          </select>
        </label>
      </div>

      <div class="row foot">
        <label class="check">
          <input v-model="shuffle" type="checkbox" />
          <span>перемешать</span>
        </label>
        <span class="spacer"></span>
        <span class="dim">{{ scopeCount }} {{ plural(scopeCount, "вопрос", "вопроса", "вопросов") }}</span>
        <button class="primary" :disabled="busy" @click="start">Начать</button>
      </div>
    </section>

    <section class="card block">
      <div class="row head">
        <h2>Работа над ошибками</h2>
        <span class="spacer"></span>
        <span v-if="appState.mistake_count > 0" class="pill bad">{{ appState.mistake_count }}</span>
        <span v-else class="pill ok">пусто</span>
      </div>
      <p class="dim">
        Сюда попадает каждый вопрос, где вы ошиблись — и в экзамене, и в тренировке.
        Вопрос уходит из списка после {{ appState.resolve_after }} верных ответов подряд.
      </p>
      <button class="start" :disabled="appState.mistake_count === 0" @click="emit('mistakes')">Открыть список</button>
    </section>

    <section class="card block">
      <div class="row head">
        <h2>Статистика</h2>
      </div>
      <p class="dim">История экзаменов, точность по разделам и группам, охват банка.</p>
      <button class="start" @click="emit('stats')">Открыть</button>
    </section>
  </div>
</template>

<style scoped>
.grid { display: grid; gap: 14px; }
.block { padding: 16px 18px; display: grid; gap: 12px; align-content: start; }
.block p { margin: 0; font-size: 13.5px; }
.exam { border-color: var(--accent); }
.big { padding: 11px 18px; font-size: 15.5px; justify-self: start; }
.start { justify-self: start; }
.head { flex-wrap: wrap; gap: 8px; }

.active {
  display: grid;
  gap: 10px;
  padding: 12px;
  background: var(--surface-2);
  border: 1px solid var(--border);
  border-left: 4px solid var(--warn);
  border-radius: 8px;
  font-size: 14px;
}
.ask { display: grid; gap: 10px; }
.actions { flex-wrap: wrap; }
.danger { background: var(--bad); border-color: var(--bad); color: var(--surface); font-weight: 600; }
.danger:hover:not(:disabled) { background: var(--bad); filter: brightness(1.08); }

.choices { display: grid; gap: 8px; }
.choice {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 9px 12px;
  background: var(--surface-2);
  border: 1px solid var(--border);
  border-radius: 8px;
  cursor: pointer;
}
.choice:has(input:checked) { border-color: var(--accent); box-shadow: inset 0 0 0 1px var(--accent); }
.choice .count { margin-left: auto; font-size: 13px; }
.choice select { margin-left: auto; max-width: 62%; min-width: 0; }

select {
  font: inherit;
  color: inherit;
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: 7px;
  padding: 5px 8px;
}

.check { display: flex; align-items: center; gap: 7px; cursor: pointer; font-size: 13.5px; }
.foot { gap: 12px; }

@media (max-width: 600px) {
  .block { padding: 14px; }
  .big { justify-self: stretch; }
  .choice { padding: 9px 10px; }
  .choice select { max-width: 70%; font-size: 14px; }
}
</style>
