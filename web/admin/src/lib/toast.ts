export type ToastType = "ok" | "err";
export type ToastItem = {
  id: number;
  text: string;
  type: ToastType;
};

type ToastListener = (item: ToastItem | null) => void;

let counter = 0;
const listeners = new Set<ToastListener>();
let hideTimer: number | null = null;

export function subscribeToast(listener: ToastListener): () => void {
  listeners.add(listener);
  return () => listeners.delete(listener);
}

function emit(item: ToastItem | null): void {
  listeners.forEach((listener) => listener(item));
}

export function toast(text: string, type: ToastType = "ok"): void {
  emit({ id: ++counter, text, type });
  if (hideTimer !== null) {
    window.clearTimeout(hideTimer);
  }
  hideTimer = window.setTimeout(() => emit(null), 2800);
}
