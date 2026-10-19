import type { SessionEvent } from "@wingman-actor/client";

export type StreamingPart = { messageID?: string; partID?: string } & (
  | { type: "text" | "reasoning"; text: string }
  | { type: "tool"; run_id: string; call_id: string }
);

export function reduceStreamingParts(parts: StreamingPart[], event: SessionEvent): StreamingPart[] {
  if (event.type === "session.message.created") {
    const message = event.data.message;
    if (message.role !== "assistant") return parts;
    return parts.filter((part) => part.messageID !== message.id);
  }
  if (event.type === "session.tool.updated" || event.type === "session.tool.input.delta") {
    const data = event.data;
    if (event.type === "session.tool.updated" && event.data.status !== "pending") return parts;
    if (!data.message_id || !data.call_id) return parts;
    if (
      parts.some(
        (part) =>
          part.type === "tool" && part.run_id === data.run_id && part.call_id === data.call_id,
      )
    )
      return parts;
    return [
      ...parts,
      {
        type: "tool",
        messageID: data.message_id,
        partID: data.part_id,
        run_id: data.run_id,
        call_id: data.call_id,
      },
    ];
  }
  if (
    event.type !== "session.text.delta" &&
    event.type !== "session.reasoning.delta" &&
    event.type !== "session.reasoning.completed" &&
    event.type !== "session.text.completed"
  )
    return parts;

  const data = event.data;
  const type = event.type.startsWith("session.reasoning.") ? "reasoning" : "text";
  const completed = "text" in data;
  const text = completed ? data.text : data.delta;
  if (!text) return parts;
  const index = parts.findIndex(
    (part) =>
      part.type === type && part.messageID === data.message_id && part.partID === data.part_id,
  );
  const previous = parts[index];
  const part: StreamingPart = {
    type,
    messageID: data.message_id,
    partID: data.part_id,
    text: !completed && previous && previous.type !== "tool" ? previous.text + text : text,
  };
  if (index < 0) return [...parts, part];
  return parts.map((current, i) => (i === index ? part : current));
}
