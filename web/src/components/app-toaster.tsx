import { Toast as ToastPrimitive } from "@base-ui/react/toast";

import {
  Toast,
  ToastClose,
  ToastDescription,
  ToastTitle,
  ToastViewport,
} from "@/components/core/toast";

function toastVariant(type: string | undefined) {
  if (type === "destructive" || type === "success") return type;
  return "default";
}

export function AppToaster() {
  const { toasts } = ToastPrimitive.useToastManager();

  return (
    <ToastViewport>
      {toasts.map((toast) => (
        <Toast key={toast.id} toast={toast} variant={toastVariant(toast.type)}>
          <div className="grid gap-1 pr-6">
            <ToastTitle />
            <ToastDescription />
          </div>
          <ToastClose aria-label="Close notification" />
        </Toast>
      ))}
    </ToastViewport>
  );
}
