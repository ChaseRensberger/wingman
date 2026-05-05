import { Button } from "@/components/core/button"
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/core/tooltip"

export function TooltipShowcase() {
  return (
    <section className="py-4 space-y-8">
      <h2 className="text-2xl font-semibold">Tooltip</h2>
      <TooltipProvider>
        <div className="flex flex-wrap gap-3">
          <Tooltip>
            <TooltipTrigger render={<Button variant="outline" />}>
              Hover me
            </TooltipTrigger>
            <TooltipContent>Add to library</TooltipContent>
          </Tooltip>
          <Tooltip>
            <TooltipTrigger render={<Button variant="outline" />}>
              Top
            </TooltipTrigger>
            <TooltipContent side="top">Tooltip on top</TooltipContent>
          </Tooltip>
          <Tooltip>
            <TooltipTrigger render={<Button variant="outline" />}>
              Bottom
            </TooltipTrigger>
            <TooltipContent side="bottom">Tooltip on bottom</TooltipContent>
          </Tooltip>
          <Tooltip>
            <TooltipTrigger render={<Button variant="outline" />}>
              Left
            </TooltipTrigger>
            <TooltipContent side="left">Tooltip on left</TooltipContent>
          </Tooltip>
          <Tooltip>
            <TooltipTrigger render={<Button variant="outline" />}>
              Right
            </TooltipTrigger>
            <TooltipContent side="right">Tooltip on right</TooltipContent>
          </Tooltip>
        </div>
      </TooltipProvider>
    </section>
  )
}
