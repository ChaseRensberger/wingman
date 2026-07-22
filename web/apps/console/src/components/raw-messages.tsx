import { useState } from "react";
import { CheckIcon, CopyIcon } from "@phosphor-icons/react";

import type { Message } from "@/lib/types";
import { formatTokenCount } from "@/lib/utils";
import { showErrorToast } from "@/lib/toast";
import { Accordion, AccordionContent, AccordionItem, AccordionTrigger } from "@wingman/core/components/core/accordion";
import { Button } from "@wingman/core/components/core/button";

function RawMessage({ index, message }: { index: number; message: Message }) {
  const [copied, setCopied] = useState(false);
  const raw = JSON.stringify(message, null, 2);
  const label = [message.role, message.origin?.provider, message.origin?.model_id].filter(Boolean).join(" / ");
  const tokenLabel = message.usage ? `${formatTokenCount(message.usage.total_tokens)} tokens` : "Not reported";

  async function copy() {
    try {
      await navigator.clipboard.writeText(raw);
      setCopied(true);
      window.setTimeout(() => setCopied(false), 1200);
    } catch (err) {
      showErrorToast(err, "Could not copy");
    }
  }

  return (
    <AccordionItem value={String(index)}>
      <AccordionTrigger className="gap-3 px-1 hover:no-underline">
        <span className="min-w-0 truncate">{label}</span>
        <span className="shrink-0 text-xs font-normal text-muted-foreground">{tokenLabel}</span>
      </AccordionTrigger>
      <AccordionContent className="px-1">
        <div className="relative">
          <Button type="button" variant="ghost" size="icon-xs" className="absolute right-2 top-2 z-10 bg-background/80" onClick={() => void copy()} aria-label={copied ? "Copied message JSON" : "Copy message JSON"} title={copied ? "Copied" : "Copy JSON"}>
            {copied ? <CheckIcon className="size-3.5" /> : <CopyIcon className="size-3.5" />}
          </Button>
          <pre className="max-h-[32rem] overflow-auto rounded-lg border bg-muted/40 p-3 pr-10 text-xs leading-5"><code>{raw}</code></pre>
        </div>
      </AccordionContent>
    </AccordionItem>
  );
}

export function RawMessages({ messages }: { messages: Message[] }) {
  if (messages.length === 0) {
    return <p className="text-sm text-muted-foreground">No persisted messages yet.</p>;
  }
  return <Accordion multiple>{messages.map((message, index) => <RawMessage key={index} index={index} message={message} />)}</Accordion>;
}
