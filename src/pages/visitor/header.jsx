import React, { useState, useContext } from 'react';
import { useNavigate } from 'react-router-dom';
import { Button } from "@/components/ui/button";
import { Menu, X, Moon, Sun, LogOut, ShieldCheck, Package } from 'lucide-react';
import Logo from '/src/assets/cackle.svg'
import LogoFallback from '/src/assets/cackle.png'
import { useTheme } from '@/components/theme-provider'
import CartDropdown from '@/pages/visitor/cart/dropdown';
import { AuthContext } from '../../context/use-auth';

const Header = () => {
  const [isMobileMenuOpen, setIsMobileMenuOpen] = useState(false);
  const { theme, setTheme } = useTheme();
  const { user, loading, signOut, activeOrganization } = useContext(AuthContext);
  const navigate = useNavigate();

  const handleSignOut = async () => {
    try {
      await signOut();
      navigate('/');
    } catch (error) {
      console.error('Error signing out:', error);
    }
  };

  const handleNavigation = (path) => {
    navigate(path);
    setIsMobileMenuOpen(false); // Close mobile menu after navigation
  };

  const AuthButtons = ({ isMobile = false }) => {
    if (loading) {
      return null;
    }

    if (user) {
      return (
        <>
          <Button
            variant="ghost"
            size={isMobile ? "sm" : "default"}
            className="text-gray-600 dark:text-slate-300 hover:text-gray-900 dark:hover:text-slate-100 hover:bg-gray-100 dark:hover:bg-slate-800"
            onClick={() => handleNavigation('/orders')}
          >
            <Package className="h-4 w-4 mr-2" />
            Orders
          </Button>
          {activeOrganization && (
            <Button
              variant="ghost"
              size={isMobile ? "sm" : "default"}
              className="text-gray-600 dark:text-slate-300 hover:text-gray-900 dark:hover:text-slate-100 hover:bg-gray-100 dark:hover:bg-slate-800"
              onClick={() => handleNavigation('/admin')}
            >
              <ShieldCheck className="h-4 w-4 mr-2" />
              Admin
            </Button>
          )}
          <Button
            onClick={handleSignOut}
            variant="ghost"
            size={isMobile ? "sm" : "default"}
            className="text-gray-600 dark:text-slate-300 hover:text-gray-900 dark:hover:text-slate-100 hover:bg-gray-100 dark:hover:bg-slate-800"
          >
            <LogOut className="h-4 w-4 mr-2" />
            Sign Out
          </Button>
        </>
      );
    }

    return (
      <>
        <Button
          variant="ghost"
          size={isMobile ? "sm" : "default"}
          className="text-gray-600 dark:text-slate-300 hover:text-gray-900 dark:hover:text-slate-100 hover:bg-gray-100 dark:hover:bg-slate-800"
          onClick={() => handleNavigation('/login')}
        >
          Log In
        </Button>
        <Button
          size={isMobile ? "sm" : "default"}
          className="bg-[#FF4848] text-white hover:bg-red-600"
          onClick={() => handleNavigation('/signup')}
        >
          Sign Up
        </Button>
      </>
    );
  };

  return (
    <header className="fixed top-0 left-0 right-0 z-50 bg-white dark:bg-slate-900 border-b border-gray-200 dark:border-slate-800 shadow-sm">
      <div className="container mx-auto px-4">
        <div className="flex items-center justify-between h-16">
          {/* Logo Section */}
          <div className="flex items-center">
  <a href="/" className="flex items-center gap-2">
    <picture>
      <source srcSet={Logo} type="image/svg+xml" />
      <img 
        src={LogoFallback}
        alt="Howler Logo"
        className="h-10 w-10 object-contain rounded-lg"
      />
    </picture>
    <span className="hidden md:block text-[#FF4848] font-bold text-3xl">
      cackle
    </span>
  </a>
</div>
          {/* Desktop Navigation */}
          <div className="hidden md:flex items-center space-x-4">
            <CartDropdown />
            <AuthButtons />
            <Button
              onClick={() => setTheme(theme === "light" ? "dark" : "light")}
              variant="ghost"
              size="icon"
              className="rounded-lg hover:bg-gray-100 dark:hover:bg-slate-800"
            >
              {theme === "light" ? (
                <Moon className="h-5 w-5 text-gray-600" />
              ) : (
                <Sun className="h-5 w-5 text-slate-300" />
              )}
            </Button>
          </div>

          {/* Mobile Navigation */}
          <div className="flex md:hidden items-center gap-2">
            <CartDropdown isMobile />
            <Button
              variant="ghost"
              size="sm"
              onClick={() => setIsMobileMenuOpen(!isMobileMenuOpen)}
              className="inline-flex items-center justify-center p-2 text-gray-600 dark:text-slate-300 hover:text-gray-900 dark:hover:text-slate-100 hover:bg-gray-100 dark:hover:bg-slate-800"
            >
              {isMobileMenuOpen ? (
                <X className="h-6 w-6" />
              ) : (
                <Menu className="h-6 w-6" />
              )}
            </Button>
          </div>
        </div>
      </div>

      {/* Mobile Menu */}
      {isMobileMenuOpen && (
        <div className="md:hidden bg-white dark:bg-slate-900 border-t border-gray-200 dark:border-slate-800">
          <div className="px-2 pt-2 pb-3 space-y-1">
            <div className="flex flex-col space-y-2 p-2">
              <AuthButtons isMobile />
              <Button
                onClick={() => setTheme(theme === "light" ? "dark" : "light")}
                variant="ghost"
                size="sm"
                className="rounded-lg hover:bg-gray-100 dark:hover:bg-slate-800 flex items-center justify-center"
              >
                {theme === "light" ? (
                  <Moon className="h-5 w-5 text-gray-600" />
                ) : (
                  <Sun className="h-5 w-5 text-slate-300" />
                )}
                <span className="ml-2">
                  {theme === "light" ? "Dark Mode" : "Light Mode"}
                </span>
              </Button>
              <select className="bg-white dark:bg-slate-800 border border-gray-200 dark:border-slate-700 rounded p-2 mt-2 text-gray-600 dark:text-slate-300 focus:border-blue-500 focus:ring-blue-500">
                <option value="en">English</option>
              </select>
            </div>
          </div>
        </div>
      )}
    </header>
  );
};

export default Header;