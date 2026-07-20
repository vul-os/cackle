import React from 'react';

/**
 * Print stylesheet shared by the ticket detail page and the tickets list.
 * Rendered as a plain inline <style> tag — no external stylesheet, no CDN —
 * so printing composes with the app's normal Tailwind classes instead of
 * duplicating them: the printed page is the SAME markup the screen shows,
 * just re-laid-out for paper. This produces nothing visible on screen; only
 * the @media print rules below take effect, and only when the browser's
 * print dialog is actually invoked (Ctrl+P / window.print()).
 */
export default function PrintStyles() {
    return (
        <style>{`
            @media print {
                @page {
                    size: A4;
                    margin: 12mm;
                }
                html, body {
                    background: #fff !important;
                }
                .print-ticket {
                    break-inside: avoid;
                    page-break-inside: avoid;
                    box-shadow: none !important;
                }
                .print-ticket + .print-ticket {
                    margin-top: 10mm;
                }
                /* Chromium/WebKit drop background colors by default when
                   printing; the ticket's colour band and the QR's white
                   backing plate are load-bearing (contrast + scannability),
                   so force them to print as authored. */
                .print-keep-color,
                .print-keep-color * {
                    -webkit-print-color-adjust: exact !important;
                    print-color-adjust: exact !important;
                    color-adjust: exact !important;
                }
            }
        `}</style>
    );
}
