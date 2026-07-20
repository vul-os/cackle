import React from 'react';
import { useLocation } from 'react-router-dom';
import { AnimatePresence, motion, useReducedMotion } from 'framer-motion';

/**
 * Wraps route content (the <Outlet/> in each layout) with a restrained
 * cross-fade + rise on navigation. Keyed on the pathname so React treats
 * each route as a distinct element and AnimatePresence can transition
 * between them.
 *
 * `useReducedMotion` (backed by `prefers-reduced-motion`) short-circuits to
 * an instant, no-transform mount — this is framer-motion's own JS-driven
 * animation loop, which the CSS `@media (prefers-reduced-motion)` block in
 * index.css cannot reach (that block only zeroes CSS transition/animation
 * durations), so it has to be handled explicitly here.
 */
const PageTransition = ({ children, className }) => {
    const location = useLocation();
    const prefersReducedMotion = useReducedMotion();

    const variants = prefersReducedMotion
        ? { initial: { opacity: 1, y: 0 }, animate: { opacity: 1, y: 0 }, exit: { opacity: 1, y: 0 } }
        : { initial: { opacity: 0, y: 8 }, animate: { opacity: 1, y: 0 }, exit: { opacity: 0, y: -8 } };

    return (
        <AnimatePresence mode="wait" initial={false}>
            <motion.div
                key={location.pathname}
                className={className}
                variants={variants}
                initial="initial"
                animate="animate"
                exit="exit"
                transition={prefersReducedMotion ? { duration: 0 } : { duration: 0.22, ease: [0.2, 0, 0, 1] }}
            >
                {children}
            </motion.div>
        </AnimatePresence>
    );
};

export default PageTransition;
