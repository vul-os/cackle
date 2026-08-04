import React from 'react';
import { Outlet } from 'react-router-dom';
import { MotionConfig } from 'framer-motion';
import PageTransition from '../motion/page-transition';
import HonestyStrip from '../honesty/honesty-strip';

// Public / visitor surface. Deliberately bare — each visitor page brings its
// own header/footer (see routes.tsx), so this layout only supplies what's
// truly shared across all of them: the reduced-motion contract for any
// framer-motion a page reaches for, a restrained cross-fade between routes so
// navigating the storefront doesn't hard-cut, and the honesty strip.
//
// The strip is mounted HERE rather than on each page for the same reason the
// palette lives in one stylesheet: a claim repeated per page is a claim that
// drifts per page, and the pages that most needed it (the landing, the in-app
// docs) were exactly the ones that never got it. Mounting it in the two
// layouts means every route in routes.tsx carries it by construction —
// scripts/check-app.mjs asserts that no route escapes them.
const BlankLayout = () => {
    return (
        <MotionConfig reducedMotion="user">
            <div className="flex min-h-screen flex-col">
                <div className="flex-1">
                    <PageTransition>
                        <Outlet />
                    </PageTransition>
                </div>
                <HonestyStrip />
            </div>
        </MotionConfig>
    );
};

export default BlankLayout;
