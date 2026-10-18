import { useEffect, useReducer, useRef, useState } from "react";
import type { Question } from "@wingman-actor/client";
import { createQuestionDraft, questionAnswers, reduceQuestionDraft } from "@/lib/questions";
import { Button } from "@wingman/core/components/core/button";
import { Card } from "@wingman/core/components/core/card";
import { Textarea } from "@wingman/core/components/core/textarea";

type Props = {
  questions: Question[];
  onReply: (id: string, status: "answered" | "dismissed", answers?: string[][]) => Promise<void>;
  onAbort?: () => void;
};

export function QuestionPanel({ questions, onReply, onAbort }: Props) {
  const question = questions[0];
  if (!question) return null;
  return (
    <div
      className="min-h-0 shrink-0 px-3 pb-3 sm:px-4 sm:pb-4"
      aria-label="Questions awaiting your answer"
    >
      <QuestionForm
        key={question.id}
        question={question}
        pendingCount={questions.length}
        onReply={onReply}
        onAbort={onAbort}
      />
    </div>
  );
}

function QuestionForm({
  question,
  pendingCount,
  onReply,
  onAbort,
}: {
  question: Question;
  pendingCount: number;
  onReply: Props["onReply"];
  onAbort: Props["onAbort"];
}) {
  const [draft, dispatch] = useReducer(
    reduceQuestionDraft,
    question.questions,
    createQuestionDraft,
  );
  const [replying, setReplying] = useState(false);
  const [error, setError] = useState("");
  const headingRef = useRef<HTMLLegendElement>(null);
  const { index, prompts } = draft;
  const { prompt, selected, custom } = prompts[index];
  const answers = questionAnswers(draft);
  const last = index === prompts.length - 1;
  const canContinue = answers[index].length > 0;
  const canSubmit = answers.every((answer) => answer.length > 0);

  useEffect(() => {
    headingRef.current?.focus();
  }, [index]);

  async function reply(status: "answered" | "dismissed") {
    if (replying || (status === "answered" && !canSubmit)) return;
    setReplying(true);
    setError("");
    try {
      await onReply(question.id, status, status === "answered" ? answers : undefined);
    } catch (error) {
      setError(error instanceof Error ? error.message : "Could not submit your answer");
    } finally {
      setReplying(false);
    }
  }

  return (
    <Card className="mx-auto max-w-4xl gap-0 overflow-hidden p-0 shadow-lg shadow-primary/5">
      <form
        className="flex max-h-[60dvh] min-h-0 flex-col text-sm"
        onSubmit={(event) => {
          event.preventDefault();
          if (last) void reply("answered");
          else if (!replying && canContinue) dispatch({ type: "next" });
        }}
      >
        <div className="flex items-center justify-between gap-3 border-b px-4 py-3">
          <span className="text-xs font-medium text-muted-foreground">
            Question {index + 1} of {prompts.length}
            {pendingCount > 1 &&
              ` · ${pendingCount - 1} more request${pendingCount > 2 ? "s" : ""} waiting`}
          </span>
        </div>
        <div className="min-h-0 overflow-y-auto p-4">
          <fieldset key={index} disabled={replying} className="w-full min-w-0 space-y-3">
            <legend ref={headingRef} tabIndex={-1} className="w-full space-y-1 outline-none">
              <span className="block text-base font-medium break-words">{prompt.question}</span>
            </legend>
            <div className="grid gap-2">
              {(prompt.options ?? []).map((option) => (
                <label
                  key={option.label}
                  className={`flex cursor-pointer items-start gap-3 rounded-lg border px-3 py-3 transition-colors focus-within:ring-2 focus-within:ring-ring ${selected.includes(option.label) ? "border-primary bg-primary/10" : "border-border hover:bg-muted/60"} ${replying ? "pointer-events-none opacity-60" : ""}`}
                >
                  <input
                    type={prompt.multiple ? "checkbox" : "radio"}
                    name={`${question.id}-${index}`}
                    checked={selected.includes(option.label)}
                    className="mt-0.5 shrink-0 accent-primary"
                    onChange={() => dispatch({ type: "select", label: option.label })}
                  />
                  <span className="min-w-0 break-words">
                    <span className="font-medium">{option.label}</span>
                    <span className="mt-0.5 block text-xs text-muted-foreground">
                      {option.description}
                    </span>
                  </span>
                </label>
              ))}
            </div>
            <Textarea
              aria-label="Type your own answer"
              value={custom}
              placeholder="Type your own answer"
              className="min-h-16 max-h-40 resize-y text-sm"
              onChange={(event) => dispatch({ type: "custom", text: event.target.value })}
            />
          </fieldset>
          {error && (
            <p role="alert" className="mt-3 text-destructive">
              {error}
            </p>
          )}
        </div>
        <div className="flex flex-wrap items-center justify-between gap-2 border-t px-4 py-3">
          <div className="flex gap-2">
            <Button
              type="button"
              variant="ghost"
              size="sm"
              disabled={replying}
              onClick={() => void reply("dismissed")}
            >
              Dismiss
            </Button>
            {onAbort && (
              <Button type="button" variant="ghost" size="sm" disabled={replying} onClick={onAbort}>
                Stop run
              </Button>
            )}
          </div>
          <div className="flex gap-2">
            {index > 0 && (
              <Button
                type="button"
                variant="outline"
                size="sm"
                disabled={replying}
                onClick={() => dispatch({ type: "back" })}
              >
                Back
              </Button>
            )}
            <Button
              type="submit"
              size="sm"
              disabled={replying || (last ? !canSubmit : !canContinue)}
            >
              {replying ? "Sending…" : last ? "Submit answers" : "Next"}
            </Button>
          </div>
        </div>
      </form>
    </Card>
  );
}
