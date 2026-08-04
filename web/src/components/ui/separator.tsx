import * as React from "react"
import * as SeparatorPrimitive from "@radix-ui/react-separator"

import { cn } from "@/lib/utils"

/**
 * Separator draws a rule between two things.
 *
 * `variant="perforated"` swaps the hairline for the product's ticket
 * tear-line — the dashed red perforation with a punched circle at each end,
 * defined in index.css. It is the same motif as the mark and the same one the
 * landing page runs on, exposed here so the rest of the UI reaches for a
 * component instead of re-deriving a dashed border every time.
 *
 * Set `--notch` on the element (or an ancestor) to whatever surface the
 * punched circles should show through to; it defaults to the page background,
 * which is right on a page and wrong inside a card.
 *
 * @param {object} props
 * @param {'horizontal'|'vertical'} [props.orientation]
 * @param {'line'|'perforated'} [props.variant]
 */
export interface SeparatorProps extends React.ComponentPropsWithoutRef<typeof SeparatorPrimitive.Root> {
  variant?: "line" | "perforated";
}

const Separator = React.forwardRef<React.ElementRef<typeof SeparatorPrimitive.Root>, SeparatorProps>((
  { className, orientation = "horizontal", decorative = true, variant = "line", ...props },
  ref
) => (
  <SeparatorPrimitive.Root
    ref={ref}
    decorative={decorative}
    orientation={orientation}
    className={cn(
      "shrink-0",
      variant === "perforated"
        ? [
            "bg-transparent",
            orientation === "horizontal"
              ? "ticket-perforation h-0 w-full"
              : "ticket-perforation-y h-full w-0",
          ]
        : [
            "bg-border",
            orientation === "horizontal" ? "h-[1px] w-full" : "h-full w-[1px]",
          ],
      className
    )}
    {...props} />
))
Separator.displayName = SeparatorPrimitive.Root.displayName

export { Separator }
