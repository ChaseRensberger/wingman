import * as React from "react"
import { cn } from "@/lib/utils"

interface AspectRatioProps extends React.ComponentProps<"div"> {
  ratio?: number
}

function AspectRatio({ ratio = 16 / 9, className, style, ...props }: AspectRatioProps) {
  return (
    <div
      data-slot="aspect-ratio"
      style={{ paddingBottom: `${(1 / ratio) * 100}%`, ...style }}
      className={cn("relative w-full", className)}
    >
      <div className="absolute inset-0" {...props} />
    </div>
  )
}

export { AspectRatio }
