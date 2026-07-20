import { cn } from '@/lib/utils';

/**
 * Base loading-skeleton primitive. Respects prefers-reduced-motion globally
 * (see index.css) — the pulse animation is disabled rather than removed, so
 * the muted block still communicates "loading" without motion.
 */
function Skeleton({ className, ...props }) {
    return <div className={cn('animate-pulse rounded-md bg-muted', className)} {...props} />;
}

/**
 * Drop-in replacement for the ad-hoc event/listing card skeletons duplicated
 * across visitor pages (image + title + meta lines).
 */
function SkeletonCard({ className }) {
    return (
        <div className={cn('overflow-hidden rounded-xl border border-border', className)}>
            <Skeleton className="aspect-[16/9] w-full rounded-none" />
            <div className="space-y-2 p-5">
                <Skeleton className="h-5 w-3/4" />
                <Skeleton className="h-4 w-1/2" />
                <Skeleton className="h-4 w-2/3" />
            </div>
        </div>
    );
}

/**
 * Drop-in replacement for the ad-hoc "3 pulsing rows" skeleton used for
 * list/table-like loading states (e.g. orders, events lists).
 */
function SkeletonListRow({ className }) {
    return <Skeleton className={cn('h-20 w-full rounded-lg', className)} />;
}

function SkeletonList({ rows = 3, className }) {
    return (
        <div className={cn('space-y-3', className)} role="status" aria-label="Loading">
            {Array.from({ length: rows }).map((_, i) => (
                <SkeletonListRow key={i} />
            ))}
        </div>
    );
}

function SkeletonCardGrid({ count = 6, className }) {
    return (
        <div className={cn('grid grid-cols-1 gap-6 sm:grid-cols-2 lg:grid-cols-3', className)} role="status" aria-label="Loading">
            {Array.from({ length: count }).map((_, i) => (
                <SkeletonCard key={i} />
            ))}
        </div>
    );
}

export { Skeleton, SkeletonCard, SkeletonListRow, SkeletonList, SkeletonCardGrid };
