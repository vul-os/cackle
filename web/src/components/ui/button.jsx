import * as React from "react"
import { Slot } from "@radix-ui/react-slot"
import { cva } from "class-variance-authority";
import { Loader2 } from "lucide-react"

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

/**
 * `loading` is a prop rather than something each page assembles, because the
 * pages that needed it were each getting a different two thirds of it right.
 * Saving an event spun the Save icon but left the button clickable-looking;
 * other submits swapped in a spinner and dropped the label, so the control
 * changed width mid-click and the next button moved under the cursor. None of
 * them told a screen reader anything at all.
 *
 * One prop, four guarantees: the button is genuinely `disabled` (a second
 * click cannot re-submit), it is `aria-busy` so the state is announced and
 * not merely drawn, the LABEL STAYS so the control keeps its width and the
 * user keeps their reading, and the spinner is `motion-safe` — under
 * prefers-reduced-motion it renders as a static glyph rather than as a
 * near-instant flicker, matching how Skeleton behaves.
 *
 * Ignored under `asChild`: Slot renders a single child element and merges
 * props onto it, so injecting a second element here would throw. A composed
 * trigger that needs a busy state should render <Button loading> directly.
 */
const Button = React.forwardRef(({ className, variant, size, asChild = false, loading = false, disabled, children, ...props }, ref) => {
  // Slot renders exactly one child and merges props onto it, so the loading
  // branch must not hand it a two-element array — not even `[false, child]`,
  // which is what `{busy && <Loader2/>}` produces when busy is false. The
  // asChild path therefore returns before any of that.
  if (asChild) {
    return (
      <Slot className={cn(buttonVariants({ variant, size, className }))} ref={ref} disabled={disabled} {...props}>
        {children}
      </Slot>
    );
  }
  return (
    <button
      className={cn(buttonVariants({ variant, size, className }))}
      ref={ref}
      disabled={disabled || loading}
      aria-busy={loading || undefined}
      {...props}
    >
      {loading && <Loader2 className="motion-safe:animate-spin" aria-hidden="true" />}
      {children}
    </button>
  );
})
Button.displayName = "Button"

export { Button, buttonVariants }
