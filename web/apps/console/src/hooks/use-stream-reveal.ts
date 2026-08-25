import { useEffect, useRef, useState } from "react";

const MIN_CHARS_PER_FRAME = 1;
const MAX_CHARS_PER_FRAME = 18;
const BACKLOG_DIVISOR = 14;

export function useStreamReveal(target: string, active: boolean) {
  const targetRef = useRef(target);
  const visibleRef = useRef("");
  const [visible, setVisible] = useState("");

  useEffect(() => {
    targetRef.current = target;
    if (!target) {
      visibleRef.current = "";
      setVisible("");
    }
  }, [target]);

  useEffect(() => {
    if (!active && !target) return;

    let frameId = 0;
    const tick = () => {
      const targetText = targetRef.current;
      const currentVisible = visibleRef.current;
      if (currentVisible.length < targetText.length) {
        const backlog = targetText.length - currentVisible.length;
        const count = Math.min(
          MAX_CHARS_PER_FRAME,
          Math.max(MIN_CHARS_PER_FRAME, Math.ceil(backlog / BACKLOG_DIVISOR)),
        );
        const next = targetText.slice(0, currentVisible.length + count);
        visibleRef.current = next;
        setVisible(next);
      }
      if (active || visibleRef.current.length < targetRef.current.length) {
        frameId = requestAnimationFrame(tick);
      }
    };

    frameId = requestAnimationFrame(tick);
    return () => cancelAnimationFrame(frameId);
  }, [active, target]);

  return visible;
}
