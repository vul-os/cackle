import React from 'react';
import { NavLink } from 'react-router-dom';
import { motion } from 'framer-motion';
import { cn } from '@/lib/utils';

export const NavItem = ({ to, icon: Icon, text, isExpanded, end = false }) => {
    return (
        <li className="list-none px-2">
            <NavLink
                to={to}
                end={end}
                className={({ isActive }) =>
                    cn(
                        'relative flex items-center gap-3 rounded-lg px-3 py-2.5 text-sm font-medium transition-colors duration-150 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-sidebar-background',
                        isActive
                            ? 'text-sidebar-primary-foreground'
                            : 'text-sidebar-muted-foreground hover:bg-sidebar-accent hover:text-sidebar-accent-foreground',
                    )
                }
            >
                {({ isActive }) => (
                    <>
                        {/* Shared layoutId so the highlight slides between items on
                            navigation instead of just popping in/out — the one bit
                            of signature motion in the console chrome. MainLayout
                            wraps this tree in <MotionConfig reducedMotion="user">,
                            so this is automatically inert for prefers-reduced-motion. */}
                        {isActive && (
                            <motion.span
                                layoutId="active-nav-pill"
                                className="absolute inset-0 rounded-lg bg-sidebar-primary"
                                transition={{ type: 'spring', stiffness: 500, damping: 35 }}
                            />
                        )}
                        <Icon className="relative z-10 h-5 w-5 shrink-0" aria-hidden="true" />
                        <span className={cn('relative z-10 truncate transition-opacity', isExpanded ? 'opacity-100' : 'opacity-0')}>
                            {text}
                        </span>
                    </>
                )}
            </NavLink>
        </li>
    );
};

export default NavItem;
