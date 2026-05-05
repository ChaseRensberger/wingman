import * as React from "react"
import { cn } from "@/lib/utils"
import { cva, type VariantProps } from "class-variance-authority"

const spinnerVariants = cva(
  "inline-block animate-spin rounded-full border-2 border-current border-t-transparent",
  {
    variants: {
      size: {
        sm: "size-4",
        default: "size-5",
        lg: "size-6",
        xl: "size-8",
      },
    },
    defaultVariants: {
      size: "default",
    },
  }
)

function Spinner({
  className,
  size,
  ...props
}: React.ComponentProps<"span"> & VariantProps<typeof spinnerVariants>) {
  return (
    <span
      data-slot="spinner"
      role="status"
      aria-label="Loading"
      className={cn(spinnerVariants({ size }), className)}
      {...props}
    />
  )
}

export { Spinner, spinnerVariants }
