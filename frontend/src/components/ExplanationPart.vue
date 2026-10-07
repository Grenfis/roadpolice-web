<script setup lang="ts">
// Одна часть пояснения (верный ответ или неверный вариант): текст, цитаты
// пунктов правил и пометка «некорректно» с необязательным комментарием.
import { ref } from "vue";
import type { ExplanationPart } from "../lib/api";
import { api } from "../lib/api";

const props = defineProps<{
  questionId: string;
  partKey: string; // "answer" или номер неверного варианта
  title: string;
  kind: "ok" | "bad";
  part: ExplanationPart;
}>();
const emit = defineEmits<{ changed: [] }>();

const editing = ref(false);
const comment = ref("");
const busy = ref(false);
const error = ref("");

function startFlag() {
  comment.value = props.part.flag?.comment ?? "";
  error.value = "";
  editing.value = true;
}

async function save() {
  busy.value = true;
  error.value = "";
  try {
    await api.setFlag(props.questionId, props.partKey, comment.value);
    editing.value = false;
    emit("changed");
  } catch (e) {
    error.value = e instanceof Error ? e.message : String(e);
  } finally {
    busy.value = false;
  }
}

async function clear() {
  busy.value = true;
  error.value = "";
  try {
    await api.clearFlag(props.questionId, props.partKey);
    emit("changed");
  } catch (e) {
    error.value = e instanceof Error ? e.message : String(e);
  } finally {
    busy.value = false;
  }
}
</script>

<template>
  <section :class="['part', kind, { flagged: !!part.flag }]">
    <div class="head">
      <strong>{{ title }}</strong>
    </div>
    <p class="text">{{ part.text }}</p>

    <blockquote v-for="r in part.refs" :key="r.unit + r.ru" class="ref">
      <div class="label">{{ r.label }}</div>
      <div class="quote">«{{ r.ru }}»</div>
      <div v-if="r.ru_source === 'model'" class="src">перевод модели — в переводе drv.am этого места нет</div>
    </blockquote>

    <div v-if="part.flag && !editing" class="flag">
      <span class="pill bad">помечено как некорректное</span>
      <span v-if="part.flag.comment" class="comment">{{ part.flag.comment }}</span>
      <span class="spacer"></span>
      <button class="ghost small" :disabled="busy" @click="startFlag">Комментарий</button>
      <button class="ghost small" :disabled="busy" @click="clear">Снять пометку</button>
    </div>

    <div v-else-if="editing" class="flag-form">
      <textarea v-model="comment" rows="2" placeholder="Что не так? (необязательно)"></textarea>
      <div class="row">
        <span class="spacer"></span>
        <button class="ghost small" :disabled="busy" @click="editing = false">Отмена</button>
        <button class="small danger" :disabled="busy" @click="save">Отметить</button>
      </div>
    </div>

    <div v-else class="row foot">
      <span class="spacer"></span>
      <button class="ghost small dim" @click="startFlag">Некорректно</button>
    </div>

    <p v-if="error" class="err">{{ error }}</p>
  </section>
</template>

<style scoped>
.part {
  display: grid;
  gap: 8px;
  padding: 10px 12px;
  border: 1px solid var(--border);
  border-left: 4px solid var(--border);
  border-radius: 8px;
  background: var(--surface);
  font-size: 14px;
  user-select: text;
}
.part.ok { border-left-color: var(--ok); }
.part.bad { border-left-color: var(--bad); }
.part.flagged { background: var(--bad-bg); }
.head { font-size: 13.5px; }
.text { margin: 0; line-height: 1.45; }

.ref {
  margin: 0;
  padding: 6px 10px;
  border-left: 3px solid var(--accent);
  background: var(--surface-2);
  border-radius: 0 6px 6px 0;
  font-size: 13px;
  line-height: 1.4;
}
.label { font-weight: 650; color: var(--text-dim); font-size: 12px; margin-bottom: 2px; }
.src { color: var(--warn); font-size: 11.5px; margin-top: 3px; }

.flag { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; font-size: 13px; }
.comment { font-style: italic; }
.flag-form { display: grid; gap: 6px; }
textarea {
  font: inherit;
  color: inherit;
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: 7px;
  padding: 6px 8px;
  resize: vertical;
  width: 100%;
}
.foot { margin-top: -4px; }
.small { padding: 4px 10px; font-size: 12.5px; min-height: 0; }
.dim { color: var(--text-dim); }
.danger { background: var(--bad); border-color: var(--bad); color: var(--surface); font-weight: 600; }
.danger:hover:not(:disabled) { background: var(--bad); filter: brightness(1.08); }
.err { color: var(--bad); font-size: 13px; margin: 0; }
</style>
