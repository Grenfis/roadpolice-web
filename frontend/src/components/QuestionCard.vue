<script setup lang="ts">
import { ref } from "vue";
import type { Question } from "../lib/api";
import { SECTION_SHORT } from "../lib/api";
import ImageZoom from "./ImageZoom.vue";

const props = withDefaults(defineProps<{
  q: Question;
  chosen?: number;
  reveal?: boolean;
  answer?: number;
  disabled?: boolean;
  showMeta?: boolean;
}>(), { chosen: 0, reveal: false, answer: 0, disabled: false, showMeta: true });

const emit = defineEmits<{ pick: [n: number] }>();

const zoom = ref(false);

function optionState(n: number): "" | "picked" | "ok" | "bad" {
  if (props.reveal) {
    if (n === props.answer) return "ok";
    if (n === props.chosen) return "bad";
    return "";
  }
  return n === props.chosen ? "picked" : "";
}
</script>

<template>
  <article class="card question">
    <div v-if="showMeta" class="meta row">
      <span class="pill">Группа {{ q.group }}</span>
      <span class="pill">{{ SECTION_SHORT[q.section] }}</span>
      <span class="spacer"></span>
      <span class="dim id">{{ q.id }}</span>
    </div>

    <h2 class="text">{{ q.text }}</h2>

    <button v-if="q.image" class="img-btn" title="Увеличить" @click="zoom = true">
      <img :src="`/img/${q.image}`" alt="Изображение к вопросу" />
    </button>

    <ul class="options">
      <li v-for="(opt, i) in q.options" :key="i">
        <button :class="['option', optionState(i + 1)]" :disabled="disabled" @click="emit('pick', i + 1)">
          <span class="num">{{ i + 1 }}</span>
          <span class="opt-text">{{ opt }}</span>
          <span v-if="reveal && i + 1 === answer" class="mark ok-mark">верно</span>
          <span v-if="reveal && i + 1 === chosen && i + 1 !== answer" class="mark bad-mark">ваш ответ</span>
        </button>
      </li>
    </ul>
  </article>

  <ImageZoom v-if="zoom && q.image" :src="`/img/${q.image}`" @close="zoom = false" />
</template>

<style scoped>
.question { padding: 16px 18px 18px; }
.meta { margin-bottom: 10px; font-size: 12.5px; flex-wrap: wrap; gap: 6px; }
.id { font-size: 11.5px; opacity: .6; }
.text { font-size: 16.5px; line-height: 1.45; font-weight: 600; }

.img-btn {
  display: block;
  width: 100%;
  margin: 14px 0 4px;
  padding: 0;
  border: 1px solid var(--border);
  border-radius: 8px;
  background: #fff;
  overflow: hidden;
  cursor: zoom-in;
}
.img-btn:hover:not(:disabled) { background: #fff; }
.img-btn img { display: block; width: 100%; height: auto; }

.options { list-style: none; margin: 14px 0 0; padding: 0; display: grid; gap: 8px; }

.option {
  display: flex;
  align-items: flex-start;
  gap: 10px;
  width: 100%;
  text-align: left;
  padding: 10px 12px;
  background: var(--surface-2);
  border: 1px solid var(--border);
  line-height: 1.4;
}
.option .num {
  flex: none;
  width: 22px;
  height: 22px;
  border-radius: 6px;
  background: var(--surface);
  border: 1px solid var(--border);
  display: grid;
  place-items: center;
  font-size: 12.5px;
  font-weight: 650;
  color: var(--text-dim);
}
.opt-text { flex: 1; }

.option.picked { border-color: var(--accent); box-shadow: inset 0 0 0 1px var(--accent); }
.option.picked .num { background: var(--accent); border-color: var(--accent); color: var(--accent-text); }

.option.ok { background: var(--ok-bg); border-color: var(--ok-border); color: var(--ok); }
.option.ok .num { background: var(--ok); border-color: var(--ok); color: var(--surface); }
.option.bad { background: var(--bad-bg); border-color: var(--bad-border); color: var(--bad); }
.option.bad .num { background: var(--bad); border-color: var(--bad); color: var(--surface); }

.mark { flex: none; font-size: 11.5px; font-weight: 650; text-transform: uppercase; letter-spacing: .03em; padding-top: 3px; }
.ok-mark { color: var(--ok); }
.bad-mark { color: var(--bad); }

@media (pointer: coarse) {
  .option:hover:not(:disabled) { background: var(--surface-2); }
  .option.ok:hover:not(:disabled) { background: var(--ok-bg); }
  .option.bad:hover:not(:disabled) { background: var(--bad-bg); }
}
@media (max-width: 600px) {
  .question { padding: 12px 12px 14px; }
  .text { font-size: 16px; }
  .option { padding: 11px 10px; }
}
</style>
