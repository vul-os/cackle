import { cn } from '@/lib/utils';

/**
 * Shared "nothing here" state — for empty lists, empty search results, empty
 * dashboards, etc. Not an error: neutral tone, dashed border, muted icon.
 *
 *   <EmptyState
 *     icon={Ticket}
 *     title="No events yet"
 *     description="Create your first event to start selling tickets."
 *     action={<Button onClick={...}>Create event</Button>}
 *   />
 */
const EmptyState = ({ icon: Icon, title, description, action, className, ...props }) => (
    <div
        role="status"
        className={cn(
            'flex flex-col items-center justify-center gap-3 rounded-xl border border-dashed border-border bg-muted/30 px-6 py-12 text-center',
            className,
        )}
        {...props}
    >
        {Icon && (
            <div className="flex h-12 w-12 items-center justify-center rounded-full bg-muted text-muted-foreground">
                <Icon className="h-6 w-6" aria-hidden="true" />
            </div>
        )}
        {title && <p className="text-base font-semibold text-foreground">{title}</p>}
        {description && <p className="max-w-sm text-sm text-muted-foreground">{description}</p>}
        {action && <div className="mt-2">{action}</div>}
    </div>
);

export { EmptyState };
