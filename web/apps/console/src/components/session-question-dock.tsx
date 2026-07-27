import { useState } from "react";

import type { QuestionRequest } from "@/lib/types";
import { Button } from "@wingman/core/components/core/button";
import { Textarea } from "@wingman/core/components/core/textarea";

export function SessionQuestionDock({ request, onReply, onDismiss }: { request: QuestionRequest; onReply: (answers: string[][]) => Promise<void>; onDismiss: () => Promise<void> }) {
  const [index, setIndex] = useState(0);
  const [answers, setAnswers] = useState<string[][]>(() => request.questions.map(() => []));
  const [custom, setCustom] = useState("");
  const [sending, setSending] = useState(false);
  const question = request.questions[index];
  const selected = answers[index] ?? [];

  function select(label: string) {
    setAnswers((current) => current.map((answer, i) => i !== index ? answer : question.multiple ? (answer.includes(label) ? answer.filter((value) => value !== label) : [...answer, label]) : [label]));
  }
  function addCustom() {
    const value = custom.trim();
    if (!value) return;
    setAnswers((current) => current.map((answer, i) => i !== index ? answer : question.multiple ? [...answer.filter((item) => item !== value), value] : [value]));
    setCustom("");
  }
  async function submit() { setSending(true); try { await onReply(answers); } finally { setSending(false); } }
  async function dismiss() { setSending(true); try { await onDismiss(); } finally { setSending(false); } }

  return <div className="shrink-0 px-3 pb-3 sm:px-4 sm:pb-4">
    <div className="mx-auto max-w-4xl rounded-xl border bg-card p-4 shadow-lg shadow-primary/10">
      <div className="flex items-center justify-between gap-3"><div><div className="text-xs font-medium text-muted-foreground">{question.header} · {index + 1} of {request.questions.length}</div><div className="mt-1 font-medium">{question.question}</div></div><div className="flex gap-1">{request.questions.map((_, i) => <span key={i} className={`size-2 rounded-full ${i === index ? "bg-primary" : answers[i]?.length ? "bg-primary/45" : "bg-muted"}`} />)}</div></div>
      <div className="mt-3 grid gap-2">{question.options.map((option) => <button key={option.label} type="button" onClick={() => select(option.label)} disabled={sending} className={`rounded-lg border px-3 py-2 text-left ${selected.includes(option.label) ? "border-primary bg-primary/10" : "hover:bg-muted/60"}`}><div className="text-sm font-medium">{option.label}</div><div className="text-xs text-muted-foreground">{option.description}</div></button>)}</div>
      {question.custom !== false && <div className="mt-3 flex gap-2"><Textarea value={custom} onChange={(event) => setCustom(event.target.value)} placeholder="Type your own answer" className="min-h-9 resize-none py-2 text-sm" disabled={sending} /><Button type="button" variant="secondary" onClick={addCustom} disabled={!custom.trim() || sending}>Add</Button></div>}
      <div className="mt-4 flex justify-between gap-2"><Button type="button" variant="ghost" onClick={() => void dismiss()} disabled={sending}>Dismiss</Button><div className="flex gap-2">{index > 0 && <Button type="button" variant="secondary" onClick={() => setIndex(index - 1)} disabled={sending}>Back</Button>}<Button type="button" onClick={() => index === request.questions.length - 1 ? void submit() : setIndex(index + 1)} disabled={sending}>{index === request.questions.length - 1 ? "Submit" : "Next"}</Button></div></div>
    </div>
  </div>;
}
