import type { CSSProperties } from "react";

import { cn } from "#lib/utils";

const cellSize = 18;
const cellRadius = 16;
const cellWidth = Math.sqrt(3) * cellSize;

const hexPoints = (centerX: number, centerY: number) =>
  Array.from({ length: 6 }, (_, index) => {
    const angle = (Math.PI / 180) * (30 + 60 * index);
    return `${(centerX + cellRadius * Math.cos(angle)).toFixed(2)},${(centerY + cellRadius * Math.sin(angle)).toFixed(2)}`;
  }).join(" ");

const cells = (
  [
    [0, 0],
    [-cellWidth / 2, -1.5 * cellSize],
    [cellWidth / 2, -1.5 * cellSize],
    [cellWidth, 0],
    [cellWidth / 2, 1.5 * cellSize],
    [-cellWidth / 2, 1.5 * cellSize],
    [-cellWidth, 0],
  ] as const
).map(([x, y], id) => ({ id, points: hexPoints(x, y) }));

type SpinnerProps = {
  size?: number;
  duration?: number;
  offOpacity?: number;
  className?: string;
  label?: string;
};

function Spinner({
  size = 48,
  duration = 4,
  offOpacity = 0.25,
  className,
  label = "Loading",
}: SpinnerProps) {
  const style: CSSProperties & { "--off": number; "--wave-cycle": string } = {
    "--off": offOpacity,
    "--wave-cycle": `${duration}s`,
    overflow: "visible",
  };

  return (
    <svg
      data-slot="spinner"
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

export { Spinner };
