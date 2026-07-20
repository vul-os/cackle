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
            className={`flex h-full flex-col bg-zinc-950 transition-all duration-300 ${isExpanded ? 'w-60' : isMobile ? 'w-0' : 'w-16'}`}
        >
            <ul className="mt-4 space-y-1">
                {navItems.map((item) => (
                    <NavItem key={item.to} {...item} isExpanded={isExpanded} />
                ))}
            </ul>
        </div>
    );
};

export default SideNav;
