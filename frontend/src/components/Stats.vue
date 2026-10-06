<script setup lang="ts">
import type { Accuracy, Stats } from "../lib/api";
import { fmtDate, fmtTime } from "../lib/api";

defineProps<{ stats: Stats }>();
const emit = defineEmits<{ home: [] }>();

function pct(a: Accuracy): number {
  return a.answered === 0 ? 0 : Math.round((a.correct / a.answered) * 100);
}
function coverage(a: Accuracy): number {
  return a.total === 0 ? 0 : Math.round((a.seen / a.total) * 100);
}
</script>

<template>
  <section class="card head">
    <div class="row">
      <h1>Статистика</h1>
      <span class="spacer"></span>
      <button class="ghost" @click="emit('home')">В меню</button>
    </div>
    <div class="row facts">
      <span class="pill">пройдено вопросов {{ stats.overall.seen }} из {{ stats.overall.total }}</span>
      <span class="pill">охват {{ coverage(stats.overall) }}%</span>
      <span class="pill">ответов {{ stats.overall.answered }}</span>
      <span :class="['pill', { ok: pct(stats.overall) >= 90, bad: stats.overall.answered > 0 && pct(stats.overall) < 90 }]">
        точность {{ pct(stats.overall) }}%
      </span>
    </div>
  </section>

  <h2 class="title">По разделам постановления</h2>
  <div class="card table">
    <div v-for="a in stats.by_section" :key="a.key" class="trow">
      <div class="label">{{ a.title }}</div>
      <div class="bars">
        <div class="bar"><div class="fill acc" :style="{ width: `${pct(a)}%` }"></div></div>
        <div class="bar"><div class="fill cov" :style="{ width: `${coverage(a)}%` }"></div></div>
      </div>
      <div class="nums mono dim">точность {{ pct(a) }}% · охват {{ a.seen }}/{{ a.total }}</div>
    </div>
  </div>

  <h2 class="title">По группам харцашара</h2>
  <div class="card table">
    <div v-for="a in stats.by_group" :key="a.key" class="trow">
      <div class="label"><strong>{{ a.key }}</strong> — {{ a.title }}</div>
      <div class="bars">
        <div class="bar"><div class="fill acc" :style="{ width: `${pct(a)}%` }"></div></div>
        <div class="bar"><div class="fill cov" :style="{ width: `${coverage(a)}%` }"></div></div>
      </div>
      <div class="nums mono dim">точность {{ pct(a) }}% · охват {{ a.seen }}/{{ a.total }}</div>
    </div>
  </div>

  <h2 class="title">История экзаменов</h2>
  <p v-if="stats.exams.length === 0" class="empty dim">Экзаменов пока не было.</p>
  <div v-else class="card table">
    <div v-for="e in stats.exams" :key="e.id" class="trow exam">
      <span :class="['pill', { ok: e.passed, bad: !e.passed }]">{{ e.passed ? "сдан" : "не сдан" }}</span>
      <strong class="mono">{{ e.correct }} / {{ e.total }}</strong>
      <span class="dim mono">{{ fmtTime(e.elapsed_sec) }}</span>
      <span v-if="e.timed_out" class="pill bad">время вышло</span>
      <span class="spacer"></span>
      <span class="dim">{{ fmtDate(e.finished_at) }}</span>
    </div>
  </div>

  <p class="legend dim">
    Верхняя полоса — точность ответов, нижняя — какая часть вопросов раздела уже
    встречалась.
  </p>
</template>

<style scoped>
.head { padding: 16px 18px; display: grid; gap: 10px; }
.facts { gap: 8px; flex-wrap: wrap; }
.title { margin: 20px 0 8px; font-size: 15px; }
.table { padding: 4px 0; }
.trow {
  display: grid;
  grid-template-columns: 1fr 160px auto;
  align-items: center;
  gap: 14px;
  padding: 9px 14px;
  border-bottom: 1px solid var(--border);
  font-size: 13.5px;
}
.trow:last-child { border-bottom: none; }
.trow.exam { display: flex; align-items: center; gap: 10px; flex-wrap: wrap; }
.label { line-height: 1.35; }
.bars { display: grid; gap: 3px; }
.bar { height: 6px; background: var(--surface-2); border-radius: 999px; overflow: hidden; }
.fill { height: 100%; border-radius: 999px; }
.fill.acc { background: var(--ok); }
.fill.cov { background: var(--accent); opacity: .65; }
.nums { font-size: 12.5px; white-space: nowrap; }
.empty { padding: 14px; }
.legend { font-size: 12.5px; margin-top: 14px; }

/* на телефоне: название на всю строку, под ним полосы и цифры */
@media (max-width: 600px) {
  .head { padding: 12px 14px; }
  h1 { font-size: 19px; }
  .trow { grid-template-columns: 1fr auto; gap: 6px 12px; padding: 9px 12px; }
  .label { grid-column: 1 / -1; }
}
</style>
