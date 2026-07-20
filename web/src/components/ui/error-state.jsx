import { AlertTriangle } from 'lucide-react';
import { cn } from '@/lib/utils';
import { Button } from '@/components/ui/button';

/**
 * Shared error state — for failed fetches, failed mutations, etc. Distinct
 * from EmptyState: destructive-tinted, optional retry action.
 *
 *   <ErrorState
 *     description="Couldn't load your orders. Check your connection and try again."
 *     onRetry={() => refetch()}
 *   />
 */
const ErrorState = ({
    icon: Icon = AlertTriangle,
    title = 'Something went wrong',
    description,
    onRetry,
    retryLabel = 'Try again',
    className,
    ...props
}) => (
    <div
        role="alert"
        className={cn(
            'flex flex-col items-center justify-center gap-3 rounded-xl border border-destructive/30 bg-destructive/5 px-6 py-12 text-center',
            className,
        )}
        {...props}
    >
        <div className="flex h-12 w-12 items-center justify-center rounded-full bg-destructive/10 text-destructive">
            <Icon className="h-6 w-6" aria-hidden="true" />
        </div>
        {title && <p className="text-base font-semibold text-foreground">{title}</p>}
        {description && <p className="max-w-sm text-sm text-muted-foreground">{description}</p>}
        {onRetry && (
            <Button variant="outline" size="sm" className="mt-1" onClick={onRetry}>
                {retryLabel}
            </Button>
        )}
    </div>
);

export { ErrorState };
