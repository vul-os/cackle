import React from 'react';
import { AlertTriangle } from 'lucide-react';
import {
    BUILD_STATUS_LABEL,
    BUILD_STATUS_DETAIL,
    CROSS_GATE_LIMIT_LABEL,
    CROSS_GATE_LIMIT_DETAIL,
} from './claims';

/**
 * The two claims the product cannot get wrong, rendered on every route.
 *
 * It is deliberately NOT dismissible and carries no state: an honesty notice
 * you can click away is an honesty notice most people never read twice. It
 * sits at the end of the layout's flow rather than pinned over the top of the
 * page, because both surfaces already own their top edge (the visitor pages
 * have a fixed header at `top-0`, the console has `TopBar`) and a fixed strip
 * would either cover one of them or need every page's padding rewritten.
 *
 * Type is `text-sm` (14px), two clear steps above the suite's 12px floor, so a
 * future wrapper that shrinks its children still cannot push it under.
 */
const HonestyStrip = ({ className = '' }) => {
    return (
        <aside
            aria-label="Build status and known limits"
            className={`border-t border-border bg-muted/60 px-4 py-3 text-sm text-muted-foreground sm:px-6 ${className}`}
        >
            <div className="mx-auto flex max-w-5xl flex-col gap-2 sm:flex-row sm:gap-6">
                <p className="flex items-start gap-2">
                    <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-primary-emphasis" aria-hidden="true" />
                    <span>
                        <strong className="font-semibold text-foreground">{BUILD_STATUS_LABEL}</strong>{' '}
                        {BUILD_STATUS_DETAIL}
                    </span>
                </p>
                <p className="sm:border-l sm:border-border sm:pl-6">
                    <strong className="font-semibold text-foreground">{CROSS_GATE_LIMIT_LABEL}</strong>{' '}
                    {CROSS_GATE_LIMIT_DETAIL}
                </p>
            </div>
        </aside>
    );
};

export default HonestyStrip;
