import React from 'react';
import {
  Home,
  CalendarDays,
  Settings
} from 'lucide-react';
import { NavItem } from './nav-item';

const navItems = [
  { to: '/home', icon: Home, text: 'Home' },
  { to: '/admin/events', icon: CalendarDays, text: 'Events' },
  { to: '/settings', icon: Settings, text: 'Settings' }
];

const SideNav = ({ isExpanded, isMobile }) => {
  return (
    <div
      className={`fixed top-16 left-0 h-[calc(100vh-4rem)] bg-gray-800 text-white shadow-md transition-all duration-300 ${
        isExpanded ? 'w-60' : isMobile ? 'w-0' : 'w-16'
      }`}
    >
      <div className="mt-9">
        <ul className="space-y-1">
          {navItems.map((item) => (
            <NavItem 
              key={item.to} 
              {...item} 
              isExpanded={isExpanded} 
              isMobile={isMobile}
            />
          ))}
        </ul>
      </div>
    </div>
  );
};

export default SideNav;