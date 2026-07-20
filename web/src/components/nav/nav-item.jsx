import React from 'react';
import { NavLink } from 'react-router-dom';
import { cn } from '@/lib/utils';

export const NavItem = ({ to, icon: Icon, text, isExpanded, end = false }) => {
    return (
        <li className="list-none px-2">
            <NavLink
                to={to}
                end={end}
                className={({ isActive }) =>
                    cn(
                        'flex items-center gap-3 rounded-lg px-3 py-2.5 text-sm font-medium text-sidebar-muted-foreground transition-colors hover:bg-sidebar-accent hover:text-sidebar-accent-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-sidebar-background',
                        isActive && 'bg-sidebar-primary text-sidebar-primary-foreground',
                    )
                }
            >
                <Icon className="h-5 w-5 shrink-0" />
                <span className={cn('truncate transition-opacity', isExpanded ? 'opacity-100' : 'opacity-0')}>{text}</span>
            </NavLink>
        </li>
    );
};

export default NavItem;
