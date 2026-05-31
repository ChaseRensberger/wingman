import { Toast as ToastPrimitive } from "@base-ui/react/toast";

export const toastManager = ToastPrimitive.createToastManager();

function errorDescription(err: unknown): string {
  return String(err instanceof Error ? err.message : err).replace(/^Error:\s*/, "");
}

export function showErrorToast(err: unknown, title = "Error") {
  toastManager.add({
    title,
    description: errorDescription(err),
    type: "destructive",
    priority: "high",
  });
}
