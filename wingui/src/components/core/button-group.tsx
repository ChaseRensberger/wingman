import * as React from "react"
import { cn } from "@/lib/utils"

function ButtonGroup({ className, ...props }: React.ComponentProps<"div">) {
  return (
    <div
      data-slot="button-group"
      className={cn(
        "flex items-center [&>*:not(:first-child)]:rounded-l-none [&>*:not(:last-child)]:rounded-r-none [&>*:not(:first-child)]:-ml-px",
        className
      )}
      {...props}
    />
  )
}

export { ButtonGroup }
