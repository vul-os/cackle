import React, { useState, useRef, useEffect, useContext } from 'react';
import { Link as RouterLink } from 'react-router-dom';
import { Menu, User, ChevronDown, Building2 } from 'lucide-react';
import { AuthContext } from '@/context/use-auth';

const TopBar = ({ onMenuClick }) => {
  const { user, signOut, organizations, activeOrganization, switchOrganization } = useContext(AuthContext);
  const [userDropdownOpen, setUserDropdownOpen] = useState(false);
  const [orgDropdownOpen, setOrgDropdownOpen] = useState(false);
  const userDropdownRef = useRef(null);
  const orgDropdownRef = useRef(null);

  // Toggle the dropdowns
  const toggleUserDropdown = () => {
    setUserDropdownOpen(!userDropdownOpen);
    setOrgDropdownOpen(false);
  };

  const toggleOrgDropdown = () => {
    setOrgDropdownOpen(!orgDropdownOpen);
    setUserDropdownOpen(false);
  };

  // Handle clicking outside the dropdowns to close them
  const handleClickOutside = (event) => {
    if (userDropdownRef.current && !userDropdownRef.current.contains(event.target)) {
      setUserDropdownOpen(false);
    }
    if (orgDropdownRef.current && !orgDropdownRef.current.contains(event.target)) {
      setOrgDropdownOpen(false);
    }
  };

  useEffect(() => {
    document.addEventListener('mousedown', handleClickOutside);
    return () => {
      document.removeEventListener('mousedown', handleClickOutside);
    };
  }, []);

  const handleSignOut = () => {
    signOut();
    setUserDropdownOpen(false);
  };

  const handleOrgSwitch = (orgId) => {
    switchOrganization(orgId);
    setOrgDropdownOpen(false);
  };

  return (
    <nav className="fixed top-0 left-0 right-0 z-50 bg-gray-900 text-white shadow-md h-16 flex justify-between items-center px-6 border-b border-gray-700">
      <div className="flex items-center">
        <button
          className="mr-2 text-white hover:text-primary focus:outline-none md:hidden"
          onClick={onMenuClick}
          aria-label="Open navigation menu"
        >
          <Menu size={24} />
        </button>
      </div>

      <div className="flex items-center gap-4 relative">
        {user && organizations?.length > 0 && (
          <div className="relative">
            <button
              onClick={toggleOrgDropdown}
              className="flex items-center gap-2 px-3 py-2 rounded hover:bg-gray-800 focus:outline-none"
              aria-label="Organization menu"
            >
              <Building2 size={20} />
              <span className="max-w-[150px] truncate">
                {activeOrganization?.name || 'Select Organization'}
              </span>
              <ChevronDown size={16} />
            </button>
            {orgDropdownOpen && (
              <div
                ref={orgDropdownRef}
                className="absolute mt-1 w-64 bg-white border border-gray-200 shadow-lg rounded-md z-10"
                style={{ top: 'calc(100% + 8px)', right: '0' }}
              >
                {organizations.map((org) => (
                  <button
                    key={org.id}
                    onClick={() => handleOrgSwitch(org.id)}
                    className={`w-full text-left px-4 py-2 hover:bg-gray-100 flex items-center gap-2 ${
                      activeOrganization?.id === org.id ? 'bg-gray-50 text-primary' : 'text-gray-800'
                    }`}
                  >
                    <Building2 size={20} className="text-gray-400" />
                    <span className="truncate">{org.name}</span>
                  </button>
                ))}
              </div>
            )}
          </div>
        )}
        
        {user ? (
          <>
            <button
              onClick={toggleUserDropdown}
              className="flex items-center gap-2 focus:outline-none hover:text-primary"
              aria-label="User menu"
            >
              <User size={24} className="text-white" />
            </button>
            {userDropdownOpen && (
              <div
                ref={userDropdownRef}
                className="absolute mt-1 w-48 bg-white border border-gray-200 shadow-lg rounded-md z-10"
                style={{ top: 'calc(100% + 16px)', right: '0' }}
              >
                <div className="px-4 py-2 text-gray-800 font-medium">
                  {user.email}
                </div>
                <button
                  onClick={handleSignOut}
                  className="w-full text-left px-4 py-2 text-gray-800 hover:bg-gray-100"
                >
                  Sign Out
                </button>
              </div>
            )}
          </>
        ) : (
          <RouterLink
            to="/login"
            className="flex items-center gap-2 hover:text-primary"
          >
            Login
          </RouterLink>
        )}
      </div>
    </nav>
  );
};

export default TopBar;