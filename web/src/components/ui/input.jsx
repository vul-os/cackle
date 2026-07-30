import * as React from 'react';

import { cn } from '@/lib/utils';

const Input = React.forwardRef(({ className, type, ...props }, ref) => {
    return (
        <input
            type={type}
            className={cn(
                // h-10 rather than h-9: 40px is the smallest comfortable
                // touch target for a field somebody fills in on a phone at a
                // ticket desk, and `text-base` below sm stops iOS Safari
                // zooming the whole page on focus (it does that under 16px).
                'flex h-10 w-full rounded-md border border-input bg-transparent px-3 py-1 text-base shadow-soft transition-[color,border-color,box-shadow] duration-150 file:border-0 file:bg-transparent file:text-sm file:font-medium placeholder:text-muted-foreground hover:border-ring/60 focus-visible:border-ring focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background disabled:cursor-not-allowed disabled:opacity-50 aria-[invalid=true]:border-destructive aria-[invalid=true]:focus-visible:ring-destructive sm:h-9 sm:text-sm',
                className,
            )}
            ref={ref}
            {...props}
        />
    );
});
Input.displayName = 'Input';

export { Input };
