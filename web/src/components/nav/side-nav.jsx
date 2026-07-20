import React from 'react';
import { Home, CalendarDays, Settings, QrCode } from 'lucide-react';
import { NavItem } from './nav-item';

const navItems = [
    { to: '/admin', icon: Home, text: 'Home', end: true },
    { to: '/admin/events', icon: CalendarDays, text: 'Events' },
    { to: '/admin/scanner', icon: QrCode, text: 'Scanner' },
    { to: '/admin/settings', icon: Settings, text: 'Settings' },
];

const SideNav = ({ isExpanded, isMobile }) => {
    return (
        <div
            className={`flex h-full w-full flex-col overflow-hidden bg-sidebar transition-opacity duration-300 ${isMobile && !isExpanded ? 'invisible opacity-0' : 'visible opacity-100'}`}
        >
            <nav aria-label="Primary" className="flex-1 px-0 py-4">
                <ul className="space-y-1">
                    {navItems.map((item) => (
                        <NavItem key={item.to} {...item} isExpanded={isExpanded} />
                    ))}
                </ul>
            </nav>
        </div>
    );
};

export default SideNav;
