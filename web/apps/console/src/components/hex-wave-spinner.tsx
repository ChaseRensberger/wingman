import type { CSSProperties } from "react";
import { cn } from "@/lib/utils";

const S = 18;
const R = 16;
const W = Math.sqrt(3) * S;

const hexPoints = (cx: number, cy: number) =>
  Array.from({ length: 6 }, (_, k) => {
    const angle = (Math.PI / 180) * (30 + 60 * k);
    return `${(cx + R * Math.cos(angle)).toFixed(2)},${(cy + R * Math.sin(angle)).toFixed(2)}`;
  }).join(" ");

const cells = (
  [
    [0, 0],
    [-W / 2, -1.5 * S],
    [W / 2, -1.5 * S],
    [W, 0],
    [W / 2, 1.5 * S],
    [-W / 2, 1.5 * S],
    [-W, 0],
  ] as const
).map(([x, y], id) => ({ id, points: hexPoints(x, y) }));

interface HexWaveSpinnerProps {
  size?: number;
  duration?: number;
  offOpacity?: number;
  className?: string;
  label?: string;
}

export function HexWaveSpinner({
  size = 48,
  duration = 4,
  offOpacity = 0.1,
  className,
  label = "Loading",
}: HexWaveSpinnerProps) {
  const style: CSSProperties & { "--off": number; "--wave-cycle": string } = {
    "--off": offOpacity,
    "--wave-cycle": `${duration}s`,
    overflow: "visible",
  };

  return (
    <svg
      data-hexwave=""
      role="status"
      aria-label={label}
      viewBox="-54 -54 108 108"
      width={size}
      height={size}
      className={cn("text-primary", className)}
      style={style}
    >
      {cells.map((cell) => (
        <polygon
          key={cell.id}
          points={cell.points}
          fill="currentColor"
          style={{
            opacity: offOpacity,
            animation: `hexwave-${cell.id} var(--wave-cycle) linear infinite`,
          }}
        />
      ))}
    </svg>
  );
}
