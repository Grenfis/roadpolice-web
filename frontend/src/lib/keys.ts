import { onBeforeUnmount, onMounted } from "vue";

// useKeydown вешает обработчик клавиш на окно, пока компонент на экране.
export function useKeydown(fn: (e: KeyboardEvent) => void) {
  onMounted(() => window.addEventListener("keydown", fn));
  onBeforeUnmount(() => window.removeEventListener("keydown", fn));
}
