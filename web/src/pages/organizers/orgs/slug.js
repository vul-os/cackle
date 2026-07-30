// Org slug preview.
//
// The SERVER is the authority: internal/orgs.Create normalises whatever it
// receives (or derives a slug from the name when none is sent) and the
// UNIQUE constraint on orgs.slug settles collisions. This function exists
// for one reason — so the organiser can SEE what their name will turn into
// before they submit, instead of discovering it afterwards on a live
// public URL.
//
// That makes divergence from the Go rule a real (if quiet) bug: the
// preview would promise one address and the server would mint another. So
// this mirrors internal/orgs.slugify deliberately and exactly, and
// slug.test.js pins the shared cases.
//
// The rule, in both places:
//   - lowercase, trimmed
//   - every run of characters outside [a-z0-9] becomes a single hyphen
//   - leading and trailing hyphens are dropped
//   - capped at 60 characters, with any hyphen the cut exposed also dropped
//   - an input with no usable characters yields "" (the caller decides what
//     to do about that; neither side invents a placeholder)

export const MAX_ORG_SLUG_LENGTH = 60;
export const MIN_ORG_SLUG_LENGTH = 2;

export function slugifyOrgName(value) {
    return (value || '')
        .toLowerCase()
        .trim()
        .replace(/[^a-z0-9]+/g, '-')
        .replace(/^-+|-+$/g, '')
        .slice(0, MAX_ORG_SLUG_LENGTH)
        .replace(/-+$/g, '');
}
