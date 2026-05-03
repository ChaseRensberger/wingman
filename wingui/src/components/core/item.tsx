import * as React from "react"
import { cn } from "@/lib/utils"

function Item({ className, ...props }: React.ComponentProps<"div">) {
  return (
    <div
      data-slot="item"
      className={cn(
        "flex items-center gap-3 rounded-lg px-3 py-2 text-sm transition-colors hover:bg-accent hover:text-accent-foreground cursor-pointer select-none",
        className
      )}
      {...props}
    />
  )
}

function ItemIcon({ className, ...props }: React.ComponentProps<"span">) {
  return (
    <span
      data-slot="item-icon"
      className={cn("shrink-0 [&_svg]:size-4 text-muted-foreground", className)}
      {...props}
    />
  )
}

function ItemContent({ className, ...props }: React.ComponentProps<"div">) {
  return (
    <div
      data-slot="item-content"
      className={cn("flex-1 min-w-0", className)}
      {...props}
    />
  )
}

function ItemLabel({ className, ...props }: React.ComponentProps<"span">) {
  return (
    <span
      data-slot="item-label"
      className={cn("block font-medium truncate", className)}
      {...props}
    />
  )
}

function ItemDescription({ className, ...props }: React.ComponentProps<"span">) {
  return (
    <span
      data-slot="item-description"
      className={cn("block text-xs text-muted-foreground truncate", className)}
      {...props}
    />
  )
}

function ItemAction({ className, ...props }: React.ComponentProps<"div">) {
  return (
    <div
      data-slot="item-action"
      className={cn("shrink-0", className)}
      {...props}
    />
  )
}

export { Item, ItemIcon, ItemContent, ItemLabel, ItemDescription, ItemAction }
