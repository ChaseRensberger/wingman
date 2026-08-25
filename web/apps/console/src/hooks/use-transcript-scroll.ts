import { useEffect, useRef, useState } from "react";

type TranscriptScrollKey = "page-down" | "page-up" | "home" | "end" | "up" | "down";

function transcriptScrollKey(
  event: Pick<KeyboardEvent, "key" | "altKey" | "ctrlKey" | "metaKey" | "shiftKey">,
): TranscriptScrollKey | undefined {
  if (event.altKey || event.ctrlKey || event.metaKey) return;
  if (event.shiftKey && event.key !== " ") return;
  switch (event.key) {
    case "PageDown":
      return "page-down";
    case "PageUp":
      return "page-up";
    case "Home":
      return "home";
    case "End":
      return "end";
    case "ArrowUp":
      return "up";
    case "ArrowDown":
      return "down";
    case " ":
      return event.shiftKey ? "page-up" : "page-down";
  }
}

function canScrollForKey(element: HTMLElement, key: TranscriptScrollKey) {
  const up = key === "up" || key === "page-up" || key === "home";
  return up
    ? element.scrollTop > 0
    : element.scrollTop + element.clientHeight < element.scrollHeight;
}

function scrollKeyOwner(root: HTMLElement, target: EventTarget | null, key: TranscriptScrollKey) {
  const element =
    target instanceof Element ? target.closest<HTMLElement>("[data-scrollable]") : undefined;
  if (!element || element === root || !root.contains(element)) return root;
  return canScrollForKey(element, key) ? element : root;
}

function isTranscriptScrollTarget(target: EventTarget | null, key: TranscriptScrollKey) {
  const element = target instanceof HTMLElement ? target : undefined;
  if (!element) return true;
  if (["INPUT", "TEXTAREA", "SELECT"].includes(element.tagName) || element.isContentEditable)
    return false;
  return (
    (key !== "page-up" && key !== "page-down") || !element.closest("button, a[href], [role=button]")
  );
}

export function useTranscriptScroll(content: unknown, resizeTarget: unknown) {
  const scrollRef = useRef<HTMLDivElement>(null);
  const scrollFrameRef = useRef<number | null>(null);
  const scrollbarIdleTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const scrollbarDragRef = useRef<{ pointerId: number; grabOffset: number } | null>(null);
  const stickToBottomRef = useRef(true);
  const [isNearBottom, setIsNearBottom] = useState(true);
  const [isHovered, setIsHovered] = useState(false);
  const [isScrolling, setIsScrolling] = useState(false);
  const [isScrollbarDragging, setIsScrollbarDragging] = useState(false);
  const [scrollbar, setScrollbar] = useState({ height: 0, top: 0 });

  function updateScrollbar() {
    const element = scrollRef.current;
    if (!element) return;
    const trackPadding = 8;
    const trackHeight = element.clientHeight - trackPadding * 2;
    if (element.scrollHeight <= element.clientHeight || trackHeight <= 0) {
      setScrollbar((current) => (current.height === 0 ? current : { height: 0, top: 0 }));
      return;
    }
    setScrollbar((current) => {
      const height = Math.max(32, (element.clientHeight / element.scrollHeight) * trackHeight);
      const maxThumbTop = trackHeight - height;
      const maxScrollTop = element.scrollHeight - element.clientHeight;
      const top =
        trackPadding + (maxScrollTop > 0 ? (element.scrollTop / maxScrollTop) * maxThumbTop : 0);
      return current.height === height && current.top === top ? current : { height, top };
    });
  }

  useEffect(() => {
    if (scrollRef.current && stickToBottomRef.current)
      scrollRef.current.scrollTop = scrollRef.current.scrollHeight;
    updateScrollbar();
  }, [content]);

  useEffect(() => {
    const element = scrollRef.current;
    if (!element) return;
    const observer = new ResizeObserver(updateScrollbar);
    observer.observe(element);
    if (element.firstElementChild instanceof HTMLElement)
      observer.observe(element.firstElementChild);
    window.addEventListener("resize", updateScrollbar);
    updateScrollbar();
    return () => {
      observer.disconnect();
      window.removeEventListener("resize", updateScrollbar);
    };
  }, [resizeTarget]);

  useEffect(
    () => () => {
      if (scrollFrameRef.current) cancelAnimationFrame(scrollFrameRef.current);
      if (scrollbarIdleTimeoutRef.current) clearTimeout(scrollbarIdleTimeoutRef.current);
    },
    [],
  );

  function reset() {
    stickToBottomRef.current = true;
    setIsNearBottom(true);
  }

  function handleScroll() {
    setIsScrolling(true);
    if (scrollbarIdleTimeoutRef.current) clearTimeout(scrollbarIdleTimeoutRef.current);
    scrollbarIdleTimeoutRef.current = window.setTimeout(() => setIsScrolling(false), 800);
    if (scrollFrameRef.current) return;
    scrollFrameRef.current = requestAnimationFrame(() => {
      scrollFrameRef.current = null;
      const element = scrollRef.current;
      if (!element) return;
      updateScrollbar();
      const nearBottom = element.scrollHeight - element.scrollTop - element.clientHeight < 80;
      stickToBottomRef.current = nearBottom;
      setIsNearBottom((current) => (current === nearBottom ? current : nearBottom));
    });
  }

  function handleKeyDown(event: React.KeyboardEvent<HTMLDivElement>) {
    const key = transcriptScrollKey(event.nativeEvent);
    if (!key || !isTranscriptScrollTarget(event.target, key)) return;
    const root = scrollRef.current;
    if (!root) return;
    const owner = scrollKeyOwner(root, event.target, key);
    if (!canScrollForKey(owner, key)) return;
    event.preventDefault();
    const behavior = window.matchMedia("(prefers-reduced-motion: reduce)").matches
      ? "auto"
      : "smooth";
    const page = owner.clientHeight * 0.8;
    switch (key) {
      case "page-down":
        owner.scrollBy({ top: page, behavior });
        break;
      case "page-up":
        owner.scrollBy({ top: -page, behavior });
        break;
      case "home":
        owner.scrollTo({ top: 0, behavior });
        break;
      case "end":
        owner.scrollTo({ top: owner.scrollHeight, behavior });
        break;
      case "up":
        owner.scrollBy({ top: -40, behavior });
        break;
      case "down":
        owner.scrollBy({ top: 40, behavior });
        break;
    }
  }

  function jumpToBottom() {
    const element = scrollRef.current;
    if (!element) return;
    reset();
    element.scrollTop = element.scrollHeight;
  }

  function handleScrollbarPointerDown(event: React.PointerEvent<HTMLDivElement>) {
    event.preventDefault();
    const thumb = event.currentTarget;
    thumb.setPointerCapture(event.pointerId);
    scrollbarDragRef.current = {
      pointerId: event.pointerId,
      grabOffset: event.clientY - thumb.getBoundingClientRect().top,
    };
    setIsScrollbarDragging(true);
  }

  function handleScrollbarPointerMove(event: React.PointerEvent<HTMLDivElement>) {
    const drag = scrollbarDragRef.current;
    const element = scrollRef.current;
    if (!drag || drag.pointerId !== event.pointerId || !element) return;
    const trackPadding = 8;
    const trackHeight = element.clientHeight - trackPadding * 2;
    const maxThumbTop = trackHeight - scrollbar.height;
    if (maxThumbTop <= 0) return;
    const thumbTop = Math.max(
      0,
      Math.min(
        event.clientY - element.getBoundingClientRect().top - trackPadding - drag.grabOffset,
        maxThumbTop,
      ),
    );
    element.scrollTop = (thumbTop / maxThumbTop) * (element.scrollHeight - element.clientHeight);
  }

  function handleScrollbarPointerUp(event: React.PointerEvent<HTMLDivElement>) {
    if (scrollbarDragRef.current?.pointerId !== event.pointerId) return;
    event.currentTarget.releasePointerCapture(event.pointerId);
    scrollbarDragRef.current = null;
    setIsScrollbarDragging(false);
  }

  return {
    scrollRef,
    scrollbar,
    isNearBottom,
    isHovered,
    isScrolling,
    isScrollbarDragging,
    setIsHovered,
    reset,
    handleScroll,
    handleKeyDown,
    jumpToBottom,
    handleScrollbarPointerDown,
    handleScrollbarPointerMove,
    handleScrollbarPointerUp,
  };
}
