import React from 'react';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import Header from '@/pages/visitor/header';
import Footer from '@/pages/visitor/landing/footer.jsx';

const DOCS = `
# Cackle Docs

Cackle is an events and ticketing platform built around one idea: **your gate works with no internet.**

## The thesis

Every ticket is a compact, Ed25519-signed capability — small enough to fit in a QR code. A gate
scanner verifies it entirely offline, against an event key pinned ahead of time. The venue's
network connection is never in the critical path of admission.

## For attendees

1. Find an event and buy a ticket — card payment via Paystack.
2. Your ticket appears under **My Tickets**, with a QR code you can screenshot, print, or scan
   straight from your phone.
3. Present it at the gate. It works even if the venue's WiFi is down.

## For organizers

1. Create an event, add ticket types, and publish when ready.
2. Before doors open, open the **Scanner** while online once — it downloads everything the gate
   needs for the whole event.
3. Unplug. Scan tickets all night. Duplicate and invalid scans are caught locally, and everything
   syncs back automatically the next time the device is online.

## Self-hosting

Cackle ships as a single static Go binary with an embedded SQLite database and this web UI
embedded via \`embed.FS\`. \`docker run -p 8080:8080 vulos/cackle\` is all it takes. See the
[README](https://github.com/vul-os/cackle) in the repository for configuration options.

## Roadmap

Signed ticket transfers and resale, venue mesh sync between multiple scanners, on-site closed-loop
payments, and DMTAP delivery (tickets arriving straight in your inbox) are on the roadmap — not
built yet.
`;

const DocsPage = () => {
    return (
        <div className="min-h-screen bg-background">
            <Header />
            <main className="mx-auto max-w-3xl px-4 pb-24 pt-24">
                <div className="prose prose-neutral dark:prose-invert max-w-none">
                    <ReactMarkdown remarkPlugins={[remarkGfm]}>{DOCS}</ReactMarkdown>
                </div>
            </main>
            <Footer />
        </div>
    );
};

export default DocsPage;
