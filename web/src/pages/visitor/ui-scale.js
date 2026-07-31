// Touch-target scale for the visitor storefront.
//
// This used to carry four class fragments (`TAP_BUTTON`, `TAP_BUTTON_SM`,
// `TAP_ICON`, `TAP_FIELD`) that reimplemented the same 44px-below-`sm` ramp
// `ui/button`, `ui/input` and `ui/select` now build in — the header used to
// say the seam existed "because ui/ is owned elsewhere". That premise no
// longer holds: `Button`'s `default` (h-11 sm:h-9), `sm` (h-11 sm:h-8) and
// `icon` (h-11 w-11 sm:h-9 sm:w-9) sizes, plus `Input` and `SelectTrigger`
// (h-11 sm:h-9), are exactly those four fragments. Passing them as a
// `className` was redundant with the primitive's own default, so all four
// were deleted rather than kept as a second name for the same ramp.
//
// What is left is the one thing that was never a primitive-size fragment:
// a raw `<a>` needs its OWN touch-target treatment, because `ui/` has no
// link component to carry one.
//
// The one deliberate exception is a link inside a running sentence — WCAG
// 2.5.8's inline exception — where growing the target would break the line
// box of the prose around it.

/** A standalone navigation link in a list — not prose. */
export const TAP_LINK = 'inline-flex min-h-[44px] min-w-[44px] items-center sm:min-h-0 sm:min-w-0';
