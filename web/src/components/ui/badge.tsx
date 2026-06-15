import * as React from "react"
import { cva, type VariantProps } from "class-variance-authority"

import { cn } from "@/lib/utils"

const badgeVariants = cva(
  "inline-flex items-center border px-2.5 py-0.5 text-xs font-semibold transition-colors focus:outline-none focus:ring-1 focus:ring-foreground",
  {
    variants: {
      variant: {
        default:
          "border-transparent bg-foreground text-background",
        secondary:
          "border-transparent bg-foreground/5 text-foreground/70",
        destructive:
          "border-transparent bg-[#e11d48] text-background",
        outline: "border-foreground/20 text-foreground",
      },
    },
    defaultVariants: {
      variant: "default",
    },
  }
)

export interface BadgeProps
  extends React.HTMLAttributes<HTMLDivElement>,
    VariantProps<typeof badgeVariants> {}

function Badge({ className, variant, ...props }: BadgeProps) {
  return (
    <div className={cn(badgeVariants({ variant }), className)} {...props} />
  )
}

export { Badge, badgeVariants }
