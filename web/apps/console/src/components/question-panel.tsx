import { useState } from "react";
import type { Question } from "@wingman-actor/client";
import { Button } from "@wingman/core/components/core/button";

type Props = {
  questions: Question[];
  onReply: (id: string, status: "answered" | "dismissed", answers?: string[][]) => Promise<void>;
};

export function QuestionPanel({ questions, onReply }: Props) {
  if (!questions.length) return null;
  return (
    <div
      className="max-h-[45dvh] shrink-0 overflow-y-auto border-t border-border"
      aria-label="Questions awaiting your answer"
    >
      <div className="mx-auto max-w-4xl space-y-4 px-4 py-3">
        {questions.map((question) => (
          <QuestionForm key={question.id} question={question} onReply={onReply} />
        ))}
      </div>
    </div>
  );
}

function QuestionForm({ question, onReply }: { question: Question; onReply: Props["onReply"] }) {
  const prompts = question.questions ?? [];
  const [selected, setSelected] = useState<string[][]>(() => prompts.map(() => []));
  const [custom, setCustom] = useState<string[]>(() => prompts.map(() => ""));
  const [replying, setReplying] = useState(false);
  const [error, setError] = useState("");
  const answers = prompts.map((prompt, i) => {
    const text = custom[i].trim();
    return text ? (prompt.multiple ? [...new Set([...selected[i], text])] : [text]) : selected[i];
  });

  async function reply(status: "answered" | "dismissed") {
    if (replying) return;
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
    <form
      className="space-y-3 text-sm"
      onSubmit={(event) => {
        event.preventDefault();
        void reply("answered");
      }}
    >
      {prompts.map((prompt, index) => (
        <fieldset key={index} disabled={replying} className="space-y-2">
          <legend className="font-medium">
            {prompt.header}: {prompt.question}
          </legend>
          <div className="flex flex-wrap gap-2">
            {(prompt.options ?? []).map((option) => (
              <label
                key={option.label}
                className="flex cursor-pointer items-start gap-2 rounded border border-border px-3 py-2"
              >
                <input
                  type={prompt.multiple ? "checkbox" : "radio"}
                  name={`${question.id}-${index}`}
                  checked={selected[index].includes(option.label)}
                  onChange={() => {
                    setSelected((previous) =>
                      previous.map((values, i) =>
                        i !== index
                          ? values
                          : prompt.multiple
                            ? values.includes(option.label)
                              ? values.filter((value) => value !== option.label)
                              : [...values, option.label]
                            : [option.label],
                      ),
                    );
                    if (!prompt.multiple)
                      setCustom((previous) =>
                        previous.map((value, i) => (i === index ? "" : value)),
                      );
                  }}
                />
                <span>
                  {option.label}
                  <span className="block text-xs text-muted-foreground">{option.description}</span>
                </span>
              </label>
            ))}
          </div>
          <label className="block space-y-1">
            <span className="text-xs text-muted-foreground">Type your own answer</span>
            <input
              className="w-full rounded border border-input bg-background px-3 py-2"
              value={custom[index]}
              onChange={(event) => {
                const text = event.target.value;
                setCustom((previous) => previous.map((value, i) => (i === index ? text : value)));
                if (!prompt.multiple && text)
                  setSelected((previous) =>
                    previous.map((values, i) => (i === index ? [] : values)),
                  );
              }}
            />
          </label>
        </fieldset>
      ))}
      {error && (
        <p role="alert" className="text-destructive">
          {error}
        </p>
      )}
      <div className="flex justify-end gap-2">
        <Button
          type="button"
          variant="outline"
          size="sm"
          disabled={replying}
          onClick={() => void reply("dismissed")}
        >
          Dismiss
        </Button>
        <Button
          type="submit"
          size="sm"
          disabled={replying || answers.some((answer) => answer.length === 0)}
        >
          {replying ? "Sending…" : "Answer"}
        </Button>
      </div>
    </form>
  );
}
