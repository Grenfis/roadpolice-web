// Тема своя у каждого браузера, поэтому живёт в localStorage, а не на сервере.
import { computed, ref } from "vue";

export type Theme = "system" | "light" | "dark";

const KEY = "theme";
const media = window.matchMedia("(prefers-color-scheme: dark)");

function load(): Theme {
  try {
    const v = localStorage.getItem(KEY);
    if (v === "light" || v === "dark" || v === "system") return v;
  } catch {
    // приватный режим или запрет хранилища — просто авто
  }
  return "system";
}

const theme = ref<Theme>(load());

function apply() {
  const dark = theme.value === "dark" || (theme.value === "system" && media.matches);
  document.documentElement.dataset.theme = dark ? "dark" : "light";
}
media.addEventListener("change", () => { if (theme.value === "system") apply(); });
apply();

export const themeLabel = computed(() =>
  theme.value === "system" ? "Тема: авто" : theme.value === "light" ? "Тема: светлая" : "Тема: тёмная",
);

export function cycleTheme() {
  theme.value = theme.value === "system" ? "light" : theme.value === "light" ? "dark" : "system";
  apply();
  try {
    localStorage.setItem(KEY, theme.value);
  } catch {
    // не сохранилось — тема продержится до перезагрузки
  }
}
