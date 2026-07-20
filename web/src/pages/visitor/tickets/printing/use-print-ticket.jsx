import { useCallback, useEffect, useRef, useState } from 'react';

/**
 * Printing is done with the browser's native window.print() against the
 * SAME document — not a cloned popup rebuilding Tailwind's utility classes
 * by hand (which is what the original app did, and which is exactly why it
 * drifted from the real design: any class the popup's hand-rolled CSS
 * didn't cover silently rendered wrong). Printing the live page means the
 * printed ticket always matches the on-screen design, fonts, and colors,
 * with zero duplication to maintain.
 *
 * `target` is `null` (nothing being printed), `'all'` (print every ticket
 * currently rendered — i.e. respecting whatever filters are active), or a
 * single ticket id. Consumers apply a `print:hidden` class to any ticket
 * card that doesn't match `target` so only the intended ticket(s) end up on
 * paper.
 */
export function usePrintTicket() {
    const [target, setTarget] = useState(null);
    const pendingPrint = useRef(false);

    // Wait one frame after `target` changes so the print:hidden classes it
    // drives have actually applied to the DOM before the print dialog reads
    // layout, then trigger the dialog.
    useEffect(() => {
        if (!pendingPrint.current) return undefined;
        pendingPrint.current = false;
        const frame = requestAnimationFrame(() => window.print());
        return () => cancelAnimationFrame(frame);
    }, [target]);

    useEffect(() => {
        const clear = () => setTarget(null);
        window.addEventListener('afterprint', clear);
        return () => window.removeEventListener('afterprint', clear);
    }, []);

    const printSingleTicket = useCallback((ticketId) => {
        pendingPrint.current = true;
        setTarget(ticketId);
    }, []);

    const printAllTickets = useCallback(() => {
        pendingPrint.current = true;
        setTarget('all');
    }, []);

    return {
        isPrinting: target !== null,
        printTarget: target,
        printSingleTicket,
        printAllTickets,
    };
}
