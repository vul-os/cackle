import React from 'react';
import { Link } from 'react-router-dom';
import { cn } from '@/lib/utils';
import { Skeleton } from '@/components/ui/skeleton';

const chipClasses = (active) =>
    cn(
        'inline-flex shrink-0 items-center gap-1.5 whitespace-nowrap rounded-full border px-4 py-2 text-sm font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2',
        active
            ? 'border-transparent bg-primary text-primary-foreground shadow'
            : 'border-border bg-card text-foreground/80 hover:bg-muted',
    );

/**
 * Horizontally-scrolling category chip row, backed by `GET /api/categories`.
 * Purely a filter convenience — pass either `getHref(slug)` (renders links,
 * for the landing page's "route into /events" pattern) or `onSelect(slug)`
 * (renders buttons, for in-page filtering on the browse page). Renders
 * nothing once loaded if there are no categories, rather than an empty
 * bar — this is a bonus surface, never a load-bearing one.
 */
const CategoryTabs = ({ categories = [], value = '', loading = false, error = false, getHref, onSelect, className }) => {
    if (error) return null;

    if (loading) {
        return (
            <div className={cn('flex gap-2 overflow-x-auto no-scrollbar', className)} aria-hidden="true">
                {Array.from({ length: 5 }).map((_, i) => (
                    <Skeleton key={i} className="h-9 w-24 shrink-0 rounded-full" />
                ))}
            </div>
        );
    }

    if (categories.length === 0) return null;

    const items = [{ slug: '', label: 'All' }, ...categories];

    return (
        <div
            className={cn('flex gap-2 overflow-x-auto no-scrollbar', className)}
            role="tablist"
            aria-label="Filter events by category"
        >
            {items.map((c) => {
                const active = value === c.slug;
                const label = c.slug ? c.label : c.label;
                const content = (
                    <>
                        <span className="capitalize">{label}</span>
                        {typeof c.count === 'number' && (
                            <span className={cn('text-xs', active ? 'text-primary-foreground/80' : 'text-muted-foreground')}>{c.count}</span>
                        )}
                    </>
                );

                if (getHref) {
                    return (
                        <Link key={c.slug || 'all'} to={getHref(c.slug)} role="tab" aria-selected={active} className={chipClasses(active)}>
                            {content}
                        </Link>
                    );
                }

                return (
                    <button
                        key={c.slug || 'all'}
                        type="button"
                        role="tab"
                        aria-selected={active}
                        onClick={() => onSelect?.(c.slug)}
                        className={chipClasses(active)}
                    >
                        {content}
                    </button>
                );
            })}
        </div>
    );
};

export default CategoryTabs;
