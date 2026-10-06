<script setup lang="ts">
import type { MistakeRow } from "../lib/api";
import { SECTION_SHORT, fmtDate, plural } from "../lib/api";

defineProps<{ rows: MistakeRow[]; resolveAfter: number }>();
const emit = defineEmits<{ practice: []; home: [] }>();
</script>

<template>
  <section class="card head">
    <div class="row">
      <h1>Список на проработку</h1>
      <span class="spacer"></span>
      <span :class="['pill', { bad: rows.length > 0, ok: rows.length === 0 }]">
        {{ rows.length }} {{ plural(rows.length, "вопрос", "вопроса", "вопросов") }}
      </span>
    </div>
    <p class="dim">
      Вопрос уходит из списка после {{ resolveAfter }} верных ответов подряд; новая ошибка
      возвращает его обратно.
    </p>
    <div class="row">
      <button class="primary" :disabled="rows.length === 0" @click="emit('practice')">Прорешать все</button>
      <span class="spacer"></span>
      <button class="ghost" @click="emit('home')">В меню</button>
    </div>
  </section>

  <p v-if="rows.length === 0" class="empty dim">Ошибок нет — список пуст.</p>
  <ul v-else class="list">
    <li v-for="r in rows" :key="r.question.id" class="card item">
      <div class="row top">
        <span class="pill">Группа {{ r.question.group }}</span>
        <span class="pill">{{ SECTION_SHORT[r.question.section] }}</span>
        <span class="spacer"></span>
        <span class="pill bad">ошибок {{ r.wrong_count }}</span>
        <span v-if="r.streak > 0" class="pill ok">верных подряд {{ r.streak }} из {{ resolveAfter }}</span>
      </div>
      <p class="text">{{ r.question.text }}</p>
      <div class="row bottom dim">
        <span v-if="r.question.image" class="pill">с картинкой</span>
        <span class="spacer"></span>
        <span>последняя ошибка {{ fmtDate(r.last_wrong_ts) }}</span>
      </div>
    </li>
  </ul>
</template>

<style scoped>
.head { padding: 16px 18px; display: grid; gap: 10px; margin-bottom: 14px; }
.head p { margin: 0; font-size: 13.5px; }
.empty { padding: 20px; text-align: center; }

.list { list-style: none; margin: 0; padding: 0; display: grid; gap: 10px; }
.item { padding: 12px 14px; display: grid; gap: 8px; }
.top { gap: 7px; flex-wrap: wrap; }
.text { margin: 0; font-size: 14.5px; line-height: 1.45; }
.bottom { font-size: 12.5px; gap: 8px; flex-wrap: wrap; }

@media (max-width: 600px) {
  .head { padding: 12px 14px; }
  h1 { font-size: 19px; }
}
</style>
