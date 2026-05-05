import * as React from "react"
import { cn } from "@/lib/utils"

function TypographyH1({ className, ...props }: React.ComponentProps<"h1">) {
  return (
    <h1
      data-slot="typography-h1"
      className={cn("scroll-m-20 text-4xl font-bold tracking-tight", className)}
      {...props}
    />
  )
}

function TypographyH2({ className, ...props }: React.ComponentProps<"h2">) {
  return (
    <h2
      data-slot="typography-h2"
      className={cn("scroll-m-20 text-3xl font-semibold tracking-tight", className)}
      {...props}
    />
  )
}

function TypographyH3({ className, ...props }: React.ComponentProps<"h3">) {
  return (
    <h3
      data-slot="typography-h3"
      className={cn("scroll-m-20 text-2xl font-semibold tracking-tight", className)}
      {...props}
    />
  )
}

function TypographyH4({ className, ...props }: React.ComponentProps<"h4">) {
  return (
    <h4
      data-slot="typography-h4"
      className={cn("scroll-m-20 text-xl font-semibold tracking-tight", className)}
      {...props}
    />
  )
}

function TypographyP({ className, ...props }: React.ComponentProps<"p">) {
  return (
    <p
      data-slot="typography-p"
      className={cn("leading-7 not-first:mt-6", className)}
      {...props}
    />
  )
}

function TypographyLead({ className, ...props }: React.ComponentProps<"p">) {
  return (
    <p
      data-slot="typography-lead"
      className={cn("text-xl text-muted-foreground", className)}
      {...props}
    />
  )
}

function TypographyLarge({ className, ...props }: React.ComponentProps<"div">) {
  return (
    <div
      data-slot="typography-large"
      className={cn("text-lg font-semibold", className)}
      {...props}
    />
  )
}

function TypographySmall({ className, ...props }: React.ComponentProps<"small">) {
  return (
    <small
      data-slot="typography-small"
      className={cn("text-sm font-medium leading-none", className)}
      {...props}
    />
  )
}

function TypographyMuted({ className, ...props }: React.ComponentProps<"p">) {
  return (
    <p
      data-slot="typography-muted"
      className={cn("text-sm text-muted-foreground", className)}
      {...props}
    />
  )
}

function TypographyCode({ className, ...props }: React.ComponentProps<"code">) {
  return (
    <code
      data-slot="typography-code"
      className={cn("relative rounded bg-muted px-[0.3rem] py-[0.2rem] font-mono text-sm font-semibold", className)}
      {...props}
    />
  )
}

export {
  TypographyH1,
  TypographyH2,
  TypographyH3,
  TypographyH4,
  TypographyP,
  TypographyLead,
  TypographyLarge,
  TypographySmall,
  TypographyMuted,
  TypographyCode,
}
