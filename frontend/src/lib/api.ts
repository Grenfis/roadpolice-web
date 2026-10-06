// Типизированный клиент HTTP API бэкенда (backend/internal/api).

export type SectionID = "law" | "pdd" | "faults" | "first_aid";

export interface Question {
  id: string;
  group: number;
  number: number; // номер внутри группы, как в харцашаре
  section: SectionID;
  text: string;
  image: string;
  options: string[];
  answer: number; // 1-based; в экзамене до сдачи приходит 0
}

export interface Section { title: string; per_ticket: number }

export interface Meta {
  category: string;
  language: string;
  source: string;
  total: number;
  sections: Record<SectionID, Section>;
  exam: { questions: number; time_minutes: number; pass_min_correct: number };
  groups: number[];
}

export interface GroupInfo { group: number; count: number; title: string }

export interface ActiveExamInfo {
  exam_id: number;
  answered: number;
  total: number;
  remaining_ms: number;
}

export interface State {
  meta: Meta;
  section_order: SectionID[];
  by_section: Record<SectionID, number>;
  groups: GroupInfo[];
  mistake_count: number;
  resolve_after: number;
  active_exam: ActiveExamInfo | null;
}

export interface ExamSession {
  exam_id: number;
  questions: Question[];
  time_minutes: number;
  pass_min_correct: number;
  lease: string;
  chosen: Record<string, number>;
  remaining_ms: number;
}

// Открытие экзамена: либо сессия, либо результат (время вышло, пока
// страница была закрыта).
export interface ExamOpen { session?: ExamSession; result?: ExamResult }

export interface ExamTick { remaining_ms: number }

export interface ReviewItem { question: Question; chosen: number; correct: boolean }

export interface ExamResult {
  exam_id: number;
  total: number;
  correct: number;
  passed: boolean;
  timed_out: boolean;
  elapsed_sec: number;
  items: ReviewItem[];
}

export interface Selection {
  scope: "all" | "group" | "section" | "ids";
  group?: number;
  section?: string;
  ids?: string[];
  shuffle?: boolean;
}

export interface Training { key: string; questions: Question[]; resume_index: number }

export interface MistakeState {
  in_list: boolean;
  wrong_count: number;
  streak: number;
  just_solved: boolean;
}

export interface AnswerResult { correct: boolean; answer: number; mistake: MistakeState }

export interface MistakeRow {
  question: Question;
  wrong_count: number;
  streak: number;
  last_wrong_ts: number;
}

export interface ExamRecord {
  id: number;
  finished_at: number;
  total: number;
  correct: number;
  passed: boolean;
  elapsed_sec: number;
  timed_out: boolean;
}

export interface Accuracy {
  key: string;
  title: string;
  total: number;
  seen: number;
  answered: number;
  correct: number;
}

export interface Stats {
  exams: ExamRecord[];
  by_section: Accuracy[];
  by_group: Accuracy[];
  overall: Accuracy;
}

// Коды ошибок экзамена, которые присылает бэкенд.
export type ErrorCode = "exam_active" | "lease_lost" | "no_exam" | "network" | "";

export class ApiError extends Error {
  constructor(message: string, readonly code: ErrorCode) {
    super(message);
  }
}

async function call<T>(method: string, path: string, body?: unknown): Promise<T> {
  let r: Response;
  try {
    r = await fetch(path, {
      method,
      headers: body === undefined ? undefined : { "Content-Type": "application/json" },
      body: body === undefined ? undefined : JSON.stringify(body),
    });
  } catch {
    throw new ApiError("нет связи с сервером", "network");
  }
  const data = await r.json().catch(() => null);
  if (!r.ok) {
    throw new ApiError(data?.error ?? `${r.status} ${r.statusText}`, data?.code ?? "");
  }
  return data as T;
}

const exam = (id: number) => `/api/exams/${id}`;

export const api = {
  getState: () => call<State>("GET", "/api/state"),

  startExam: () => call<ExamSession>("POST", "/api/exams"),
  resumeExam: (id: number) => call<ExamOpen>("POST", `${exam(id)}/resume`),
  answerExam: (id: number, lease: string, questionID: string, chosen: number) =>
    call<ExamTick>("PUT", `${exam(id)}/answers`, { lease, question_id: questionID, chosen }),
  examStatus: (id: number, lease: string) =>
    call<ExamTick>("GET", `${exam(id)}/status?lease=${encodeURIComponent(lease)}`),
  finishExam: (id: number, lease: string) => call<ExamResult>("POST", `${exam(id)}/finish`, { lease }),
  abandonExam: (id: number) => call<object>("DELETE", exam(id)),
  // Пауза при закрытии страницы: обычный fetch браузер может оборвать,
  // sendBeacon доставляется и после выгрузки.
  pauseExam: (id: number, lease: string) =>
    navigator.sendBeacon(`${exam(id)}/pause`,
      new Blob([JSON.stringify({ lease })], { type: "application/json" })),

  startTraining: (sel: Selection) => call<Training>("POST", "/api/training", sel),
  saveTrainingPosition: (key: string, index: number) =>
    call<object>("PUT", "/api/training/position", { key, index }),
  recordAnswer: (id: string, chosen: number, mode: "training" | "mistakes") =>
    call<AnswerResult>("POST", "/api/answers", { question_id: id, chosen, mode }),
  getMistakes: () => call<MistakeRow[]>("GET", "/api/mistakes"),
  getStats: () => call<Stats>("GET", "/api/stats"),
};

export const SECTION_SHORT: Record<SectionID, string> = {
  law: "Закон о БДД",
  pdd: "ПДД",
  faults: "Неисправности",
  first_aid: "Первая помощь",
};

export function plural(n: number, one: string, few: string, many: string): string {
  const mod10 = n % 10;
  const mod100 = n % 100;
  if (mod10 === 1 && mod100 !== 11) return one;
  if (mod10 >= 2 && mod10 <= 4 && (mod100 < 12 || mod100 > 14)) return few;
  return many;
}

export function fmtTime(sec: number): string {
  const m = Math.floor(sec / 60);
  const s = sec % 60;
  return `${m}:${String(s).padStart(2, "0")}`;
}

export function fmtDate(ts: number): string {
  return new Date(ts * 1000).toLocaleString("ru-RU", {
    day: "2-digit", month: "2-digit", year: "numeric", hour: "2-digit", minute: "2-digit",
  });
}
