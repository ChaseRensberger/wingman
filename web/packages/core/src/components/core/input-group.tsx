import * as React from "react"
import { cn } from "#lib/utils"

function InputGroup({ className, ...props }: React.ComponentProps<"div">) {
  return (
    <div
      data-slot="input-group"
      className={cn("relative flex items-center", className)}
      {...props}
    />
  )
}

function InputGroupAddon({ className, ...props }: React.ComponentProps<"span">) {
  return (
    <span
      data-slot="input-group-addon"
      className={cn(
        "inline-flex h-9 items-center justify-center border border-input bg-muted px-2.5 text-sm text-muted-foreground first:rounded-l-[var(--radius)] first:border-r-0 last:rounded-r-[var(--radius)] last:border-l-0",
        className
      )}
      {...props}
    />
  )
}

function InputGroupText({ className, ...props }: React.ComponentProps<"span">) {
  return (
    <span
      data-slot="input-group-text"
      className={cn("text-sm", className)}
      {...props}
    />
  )
}

export { InputGroup, InputGroupAddon, InputGroupText }
