import React from 'react';
import { Outlet } from 'react-router-dom';
import { MotionConfig } from 'framer-motion';
import PageTransition from '../motion/page-transition';

// Public / visitor surface. Deliberately bare — each visitor page brings its
// own header/footer (see routes.jsx), so this layout only supplies what's
// truly shared across all of them: the reduced-motion contract for any
// framer-motion a page reaches for, and a restrained cross-fade between
// routes so navigating the storefront doesn't hard-cut.
const BlankLayout = () => {
    return (
        <MotionConfig reducedMotion="user">
            <PageTransition>
                <Outlet />
            </PageTransition>
        </MotionConfig>
    );
};

export default BlankLayout;
