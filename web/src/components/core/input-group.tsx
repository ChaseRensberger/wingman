import * as React from "react"
import { cn } from "@/lib/utils"

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
        "inline-flex items-center justify-center px-2.5 text-sm text-muted-foreground border border-input bg-muted first:rounded-l-lg first:border-r-0 last:rounded-r-lg last:border-l-0",
        "h-8",
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
