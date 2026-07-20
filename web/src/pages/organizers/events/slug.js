// internal/events.Service.Create requires a globally-unique slug and does
// not generate one server-side (see internal/events/events.go) — every
// client-side path that creates an event (the wizard, "duplicate") needs to
// mint one. A short random suffix keeps collisions practically impossible
// without a round-trip to check uniqueness first.
export function slugify(title) {
    const base =
        (title || 'event')
            .toLowerCase()
            .trim()
            .replace(/[^a-z0-9]+/g, '-')
            .replace(/^-+|-+$/g, '')
            .slice(0, 60) || 'event';
    const suffix = Math.random().toString(36).slice(2, 8);
    return `${base}-${suffix}`;
}
