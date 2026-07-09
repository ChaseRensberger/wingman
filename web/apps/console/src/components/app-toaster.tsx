import { Toast as ToastPrimitive } from "@base-ui/react/toast";
import { CheckIcon, CopyIcon } from "@phosphor-icons/react";
import { useState } from "react";

import {
  Toast,
  ToastClose,
  ToastDescription,
  ToastTitle,
  ToastViewport,
} from "@wingman/core/components/core/toast";
import { Button } from "@wingman/core/components/core/button";

function toastVariant(type: string | undefined) {
  if (type === "destructive" || type === "success") return type;
  return "default";
}

function toastText(toast: ToastPrimitive.Root.ToastObject) {
  return [toast.title, toast.description]
    .map((part) => (typeof part === "string" ? part : ""))
    .filter(Boolean)
    .join("\n");
}

function ToastCopyButton({ toast }: { toast: ToastPrimitive.Root.ToastObject }) {
  const [copied, setCopied] = useState(false);
  const text = toastText(toast);
  if (!text) return null;

  async function copy() {
    await navigator.clipboard.writeText(text);
    setCopied(true);
    window.setTimeout(() => setCopied(false), 1200);
  }

  return (
    <Button
      type="button"
      variant="ghost"
      size="icon-xs"
      className="absolute right-8 top-2 text-foreground/50 hover:text-foreground"
      onClick={() => void copy()}
      aria-label={copied ? "Copied notification text" : "Copy notification text"}
      title={copied ? "Copied" : "Copy"}
      data-base-ui-swipe-ignore
    >
      {copied ? <CheckIcon className="size-4" /> : <CopyIcon className="size-4" />}
    </Button>
  );
}

export function AppToaster() {
  const { toasts } = ToastPrimitive.useToastManager();

  return (
    <ToastViewport>
      {toasts.map((toast) => (
        <Toast key={toast.id} toast={toast} variant={toastVariant(toast.type)}>
          <div className="grid gap-1 pr-14">
            <ToastTitle />
            <ToastDescription />
          </div>
          <ToastCopyButton toast={toast} />
          <ToastClose aria-label="Close notification" />
        </Toast>
      ))}
    </ToastViewport>
  );
}
