import { cva } from "class-variance-authority";

export const badgeVariants = cva(
  "inline-flex w-fit shrink-0 items-center gap-1 whitespace-nowrap rounded-sm border px-2 py-0.5 text-xs font-medium [&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg:not([class*='size-'])]:size-3 data-[icon=inline-start]:pl-1.5 data-[icon=inline-end]:pr-1.5",
  {
    variants: {
      variant: {
        default: "border-primary/20 bg-primary/15 text-primary",
        secondary: "border-secondary bg-secondary text-secondary-foreground",
        destructive: "border-destructive/20 bg-destructive/15 text-destructive",
        outline: "text-foreground",
        ghost: "border-muted bg-muted text-muted-foreground",
      },
    },
    defaultVariants: {
      variant: "default",
    },
  },
);
