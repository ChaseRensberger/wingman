import { Collapsible as CollapsiblePrimitive } from "@base-ui/react/collapsible"
import { CaretDownIcon, CaretUpIcon } from "@phosphor-icons/react"

import { cn } from "@/lib/utils"

function Collapsible({ className, ...props }: CollapsiblePrimitive.Root.Props) {
  return (
    <CollapsiblePrimitive.Root
      data-slot="collapsible"
      className={cn("w-full", className)}
      {...props}
    />
  )
}

function CollapsibleTrigger({
  className,
  children,
  ...props
}: CollapsiblePrimitive.Trigger.Props) {
  return (
    <CollapsiblePrimitive.Trigger
      data-slot="collapsible-trigger"
      className={cn(
        "group/collapsible-trigger flex w-full items-center justify-between rounded-lg border border-transparent py-2.5 text-left text-sm font-medium transition-all outline-none hover:underline focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50 disabled:pointer-events-none disabled:opacity-50",
        className
      )}
      {...props}
    >
      {children}
      <CaretDownIcon className="pointer-events-none ml-auto size-4 shrink-0 text-muted-foreground group-data-panel-open/collapsible-trigger:hidden" />
      <CaretUpIcon className="pointer-events-none ml-auto hidden size-4 shrink-0 text-muted-foreground group-data-panel-open/collapsible-trigger:inline" />
    </CollapsiblePrimitive.Trigger>
  )
}

function CollapsibleContent({
  className,
  children,
  ...props
}: CollapsiblePrimitive.Panel.Props) {
  return (
    <CollapsiblePrimitive.Panel
      data-slot="collapsible-content"
      className="overflow-hidden text-sm data-open:animate-accordion-down data-closed:animate-accordion-up"
      {...props}
    >
      <div
        className={cn(
          "h-(--collapsible-panel-height) pb-2.5 data-ending-style:h-0 data-starting-style:h-0",
          className
        )}
      >
        {children}
      </div>
    </CollapsiblePrimitive.Panel>
  )
}

export { Collapsible, CollapsibleTrigger, CollapsibleContent }
