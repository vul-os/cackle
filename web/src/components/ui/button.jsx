import * as React from "react"
import { Slot } from "@radix-ui/react-slot"
import { cva } from "class-variance-authority";

import { cn } from "@/lib/utils"

// The focus ring is RED and it is `ring-offset-background`, not a bare ring.
// That offset is load-bearing rather than decorative: the primary button is
// ITSELF red, so a red ring drawn flush against it would be invisible on the
// one control the keyboard lands on most. The page-coloured gap is what makes
// the indicator readable on a red fill and on paper alike.
//
// Every fill below carries an ink measured against it in contrast.test.js.
// The pairing is also the product's danger affordance: a RED fill with INK
// type is the primary action, a CRIMSON fill with WHITE type destroys
// something. Two red buttons, never mistakable, even for someone who reads
// the two hues as one colour.
const buttonVariants = cva(
  "inline-flex items-center justify-center gap-2 whitespace-nowrap rounded-md text-sm font-medium transition-[color,background-color,border-color,box-shadow,transform] duration-150 ease-emphasized focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background disabled:pointer-events-none disabled:opacity-50 disabled:shadow-none active:scale-[0.98] motion-reduce:transition-colors motion-reduce:active:scale-100 [&_svg]:pointer-events-none [&_svg]:size-4 [&_svg]:shrink-0",
  {
    variants: {
      variant: {
        default:
          "bg-primary text-primary-foreground shadow-soft hover:bg-primary/90 hover:shadow-elevated active:bg-primary",
        destructive:
          "bg-destructive text-destructive-foreground shadow-soft hover:bg-destructive/90 hover:shadow-elevated active:bg-destructive",
        outline:
          "border border-input bg-background shadow-soft hover:border-primary hover:bg-accent hover:text-accent-foreground active:bg-accent",
        secondary:
          "bg-secondary text-secondary-foreground shadow-soft hover:bg-secondary/70 active:bg-secondary",
        ghost: "hover:bg-accent hover:text-accent-foreground active:bg-accent",
        link: "text-primary-emphasis underline-offset-4 hover:underline active:scale-100",
      },
      size: {
        default: "h-9 px-4 py-2",
        sm: "h-8 rounded-md px-3 text-xs",
        lg: "h-11 rounded-md px-8 text-base",
        xl: "h-12 rounded-lg px-10 text-base font-semibold",
        icon: "h-9 w-9",
      },
    },
    defaultVariants: {
      variant: "default",
      size: "default",
    },
  }
)

const Button = React.forwardRef(({ className, variant, size, asChild = false, ...props }, ref) => {
  const Comp = asChild ? Slot : "button"
  return (
    (<Comp
      className={cn(buttonVariants({ variant, size, className }))}
      ref={ref}
      {...props} />)
  );
})
Button.displayName = "Button"

export { Button, buttonVariants }
