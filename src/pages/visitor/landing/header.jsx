import React, { useState } from 'react';
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Menu, X } from 'lucide-react';
import Logo from '/src/assets/cackle.svg'
import LogoFallback from '/src/assets/cackle.png'
import { useTheme } from '@/components/theme-provider'
import { Moon, Sun } from "lucide-react"

const Header = () => {
  const [isMobileMenuOpen, setIsMobileMenuOpen] = useState(false);
  const { theme, setTheme } = useTheme()

  return (
    <header className="fixed top-0 left-0 right-0 z-50 bg-slate-900 border-b border-slate-800 shadow-sm">
      <div className="container mx-auto px-4">
        <div className="flex items-center justify-between h-16">
          {/* Logo Section */}
          <div className="flex items-center">
            <div className="flex items-center gap-2">
              <a href="/" className="block">
                <picture>
                  <source srcSet={Logo} type="image/svg+xml" />
                  <img 
                    src={LogoFallback}
                    alt="Howler Logo"
                    className="h-10 w-10 object-contain rounded-lg"
                  />
                </picture>
              </a>
              <span className="hidden md:block text-[#FF4848] font-bold text-3xl">
                cackle
              </span>
            </div>
          </div>

          {/* Language Selector and Auth Buttons */}
          <div className="hidden md:flex items-center space-x-4">
            <Button
              variant="ghost"
              size="sm"
              className="text-slate-300 hover:text-slate-100 hover:bg-slate-800"
            >
              Log In
            </Button>
            <Button
              size="sm"
              className="bg-[#FF4848] text-white hover:bg-red"
            >
              Sign Up
            </Button>
            <Button
                onClick={() => setTheme(theme === "light" ? "dark" : "light")}
                className="rounded-lg p-2 hover:bg-gray-100 dark:hover:bg-gray-800"
              >
                {theme === "light" ? (
                  <Moon className="h-5 w-5" />
                ) : (
                  <Sun className="h-5 w-5" />
                )}
              </Button>
          </div>

          {/* Mobile Menu Button */}
          <div className="flex md:hidden">
            <Button
              variant="ghost"
              size="sm"
              onClick={() => setIsMobileMenuOpen(!isMobileMenuOpen)}
              className="inline-flex items-center justify-center p-2 text-slate-300 hover:text-slate-100 hover:bg-slate-800"
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
        <div className="md:hidden bg-slate-900 border-t border-slate-800">
          <div className="px-2 pt-2 pb-3 space-y-1">
            <div className="flex flex-col space-y-2 p-2">
              <Button
                variant="ghost"
                size="sm"
                className="text-slate-300 hover:text-slate-100 hover:bg-slate-800"
              >
                Log In
              </Button>
              <Button
                size="sm"
                className="bg-[#FF4848] text-white hover:bg-red"
              >
                Sign Up
              </Button>
              <Button
                onClick={() => setTheme(theme === "light" ? "dark" : "light")}
                className="rounded-lg p-2 hover:bg-gray-100 dark:hover:bg-gray-800"
              >
                {theme === "light" ? (
                  <Moon className="h-5 w-5" />
                ) : (
                  <Sun className="h-5 w-5" />
                )}
              </Button>
              <select className="bg-slate-800 border border-slate-700 rounded p-2 mt-2 text-slate-300 focus:border-blue-500 focus:ring-blue-500">
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