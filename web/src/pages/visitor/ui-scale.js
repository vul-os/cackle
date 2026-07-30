// Touch-target scale for the visitor storefront.
//
// The shared `ui/button` scale tops out at 36px (`default`) and 32px (`sm`),
// and `ui/input` at 40px. Those are pointer sizes. Every one of these pages
// is bought on a phone — the audit at 390px found 115 distinct controls
// under 44x44 across the funnel, including the quantity steppers, the
// checkout submit and the whole footer nav.
//
// 44x44 is the floor here (WCAG 2.5.5 / the platform HIG minimum), applied
// only below `sm` so a mouse-driven desktop still gets the tighter,
// better-proportioned component scale. These are class fragments rather
// than component variants because ui/ is owned elsewhere; passing a
// className is the seam that exists.
//
// The one deliberate exception is a link inside a running sentence — WCAG
// 2.5.8's inline exception — where growing the target would break the line
// box of the prose around it.

/** A normal button (`size="default"`, h-9) that is also a phone tap target. */
export const TAP_BUTTON = 'h-11 sm:h-9';

/** A compact button (`size="sm"`, h-8) that is also a phone tap target. */
export const TAP_BUTTON_SM = 'h-11 sm:h-8';

/** An icon-only square control. */
export const TAP_ICON = 'h-11 w-11 sm:h-9 sm:w-9';

/** A text field or select trigger (h-10 by default). */
export const TAP_FIELD = 'h-11 sm:h-10';

/** A standalone navigation link in a list — not prose. */
export const TAP_LINK = 'inline-flex min-h-[44px] min-w-[44px] items-center sm:min-h-0 sm:min-w-0';
