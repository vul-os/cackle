// Fallback event category list shown when GET /api/categories has nothing
// to offer yet (a brand-new org with no events, or the endpoint failing —
// categories are a filter convenience, never a hard requirement to create
// an event). Organisers can still type any label; these just seed sensible
// choices and keep slugs consistent with what the public browse/landing
// category filter (owned by the visitor pages) expects.
export const FALLBACK_CATEGORIES = [
    { slug: 'music', label: 'Music' },
    { slug: 'nightlife', label: 'Nightlife & Club' },
    { slug: 'arts-theatre', label: 'Arts & Theatre' },
    { slug: 'comedy', label: 'Comedy' },
    { slug: 'sports-fitness', label: 'Sports & Fitness' },
    { slug: 'food-drink', label: 'Food & Drink' },
    { slug: 'conference', label: 'Conference & Business' },
    { slug: 'workshop', label: 'Workshop & Class' },
    { slug: 'community', label: 'Community & Culture' },
    { slug: 'family', label: 'Family & Kids' },
    { slug: 'other', label: 'Other' },
];

/**
 * Merge server-derived categories (which carry a live `count`) with the
 * fallback list (so a new org always has something to pick from), de-duped
 * by slug, server entries winning on conflict.
 */
export function mergeCategories(serverCategories) {
    const bySlug = new Map(FALLBACK_CATEGORIES.map((c) => [c.slug, c]));
    for (const c of serverCategories ?? []) {
        if (c?.slug) bySlug.set(c.slug, { slug: c.slug, label: c.label || c.slug, count: c.count });
    }
    return Array.from(bySlug.values());
}

export function categoryLabel(slug) {
    return FALLBACK_CATEGORIES.find((c) => c.slug === slug)?.label ?? slug;
}
