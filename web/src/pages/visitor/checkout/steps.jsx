import React from 'react';
import { Check } from 'lucide-react';
import { cn } from '@/lib/utils';

// The spine of the buying flow, shown on the cart, the checkout and the
// confirmation so a buyer can always see how many steps are left and which
// one they are on. Four steps, because four is what there actually are —
// inventing a fifth to look thorough would be its own small lie.
//
// The connector between the steps is the ticket perforation, dashed and in
// the brand red. This is the one place on the funnel where the motif is
// carrying meaning rather than decorating: the line the steps sit on is the
// line you tear along, and the thing you walk away with at the end of it is
// a stub.
export const STEPS = [
    { key: 'cart', label: 'Cart' },
    { key: 'details', label: 'Your details' },
    { key: 'pay', label: 'Pay' },
    { key: 'ticket', label: 'Ticket' },
];

/**
 * @param {object} props
 * @param {'cart'|'details'|'pay'|'ticket'} props.current
 */
export default function CheckoutSteps({ current, className }) {
    const currentIndex = Math.max(0, STEPS.findIndex((s) => s.key === current));

    return (
        <nav aria-label="Checkout progress" className={cn('w-full', className)}>
            <ol className="flex items-start">
                {STEPS.map((step, i) => {
                    const done = i < currentIndex;
                    const active = i === currentIndex;
                    return (
                        <li key={step.key} className="flex min-w-0 flex-1 flex-col items-center last:flex-none">
                            <div className="flex w-full items-center">
                                <span
                                    aria-hidden="true"
                                    className={cn(
                                        'h-0 flex-1 border-t-2 border-dashed',
                                        i === 0 ? 'invisible' : done || active ? 'border-primary/60' : 'border-border',
                                    )}
                                />
                                <span
                                    className={cn(
                                        'flex h-8 w-8 shrink-0 items-center justify-center rounded-full border-2 text-xs font-black tabular-nums transition-colors',
                                        done && 'border-primary bg-primary text-primary-foreground',
                                        active && 'border-primary bg-background text-primary-emphasis',
                                        !done && !active && 'border-border bg-background text-muted-foreground',
                                    )}
                                    aria-current={active ? 'step' : undefined}
                                >
                                    {done ? <Check className="h-4 w-4" aria-hidden="true" /> : i + 1}
                                    <span className="sr-only">
                                        {done ? 'completed: ' : active ? 'current step: ' : 'upcoming: '}
                                    </span>
                                </span>
                                <span
                                    aria-hidden="true"
                                    className={cn(
                                        'h-0 flex-1 border-t-2 border-dashed',
                                        i === STEPS.length - 1 ? 'invisible' : done ? 'border-primary/60' : 'border-border',
                                    )}
                                />
                            </div>
                            {/* 12px is the suite floor and this label sits on
                                it exactly — it is a wayfinding aid, never the
                                only thing telling you where you are. */}
                            <span
                                className={cn(
                                    'mt-2 px-1 text-center text-xs leading-tight',
                                    active ? 'font-bold text-foreground' : 'font-medium text-muted-foreground',
                                )}
                            >
                                {step.label}
                            </span>
                        </li>
                    );
                })}
            </ol>
        </nav>
    );
}
