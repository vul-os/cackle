import React, { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { Button } from '@/components/ui/button';
import { Menu, X, Moon, Sun, LogOut, ShieldCheck, Package } from 'lucide-react';
import { useTheme } from '@/components/theme-provider';
import CartDropdown from '@/pages/visitor/cart/dropdown';
import { useAuth } from '@/context/use-auth';
import Logo from '/cackle.svg';

const Header = ({ className = '' }) => {
    const [isMobileMenuOpen, setIsMobileMenuOpen] = useState(false);
    const { theme, setTheme } = useTheme();
    const { user, loading, signOut } = useAuth();
    const navigate = useNavigate();

    const handleSignOut = async () => {
        await signOut();
        navigate('/');
    };

    const handleNavigation = (path) => {
        navigate(path);
        setIsMobileMenuOpen(false);
    };

    const AuthButtons = ({ isMobile = false }) => {
        if (loading) return null;

        if (user) {
            return (
                <>
                    <Button variant="ghost" size={isMobile ? 'sm' : 'default'} onClick={() => handleNavigation('/orders')}>
                        <Package className="mr-2 h-4 w-4" />
                        Orders
                    </Button>
                    <Button variant="ghost" size={isMobile ? 'sm' : 'default'} onClick={() => handleNavigation('/admin')}>
                        <ShieldCheck className="mr-2 h-4 w-4" />
                        Organizer
                    </Button>
                    <Button variant="ghost" size={isMobile ? 'sm' : 'default'} onClick={handleSignOut}>
                        <LogOut className="mr-2 h-4 w-4" />
                        Sign Out
                    </Button>
                </>
            );
        }

        return (
            <>
                <Button variant="ghost" size={isMobile ? 'sm' : 'default'} onClick={() => handleNavigation('/login')}>
                    Log In
                </Button>
                <Button size={isMobile ? 'sm' : 'default'} onClick={() => handleNavigation('/signup')}>
                    Sign Up
                </Button>
            </>
        );
    };

    return (
        <header className={`fixed left-0 right-0 top-0 z-50 border-b border-border bg-background/90 backdrop-blur-md ${className}`}>
            <div className="container mx-auto px-4">
                <div className="flex h-16 items-center justify-between">
                    <a href="/" className="flex items-center gap-2">
                        <img src={Logo} alt="Cackle" className="h-9 w-9" />
                        <span className="font-display text-2xl font-black tracking-tight text-primary">cackle</span>
                    </a>

                    <div className="hidden items-center gap-2 md:flex">
                        <CartDropdown />
                        <AuthButtons />
                        <Button
                            onClick={() => setTheme(theme === 'dark' ? 'light' : 'dark')}
                            variant="ghost"
                            size="icon"
                            aria-label="Toggle theme"
                        >
                            {theme === 'dark' ? <Sun className="h-5 w-5" /> : <Moon className="h-5 w-5" />}
                        </Button>
                    </div>

                    <div className="flex items-center gap-2 md:hidden">
                        <CartDropdown isMobile />
                        <Button variant="ghost" size="sm" onClick={() => setIsMobileMenuOpen(!isMobileMenuOpen)} aria-label="Toggle menu">
                            {isMobileMenuOpen ? <X className="h-6 w-6" /> : <Menu className="h-6 w-6" />}
                        </Button>
                    </div>
                </div>
            </div>

            {isMobileMenuOpen && (
                <div className="border-t border-border bg-background md:hidden">
                    <div className="flex flex-col gap-2 p-4">
                        <AuthButtons isMobile />
                        <Button
                            onClick={() => setTheme(theme === 'dark' ? 'light' : 'dark')}
                            variant="ghost"
                            size="sm"
                            className="justify-center"
                        >
                            {theme === 'dark' ? <Sun className="mr-2 h-5 w-5" /> : <Moon className="mr-2 h-5 w-5" />}
                            {theme === 'dark' ? 'Light Mode' : 'Dark Mode'}
                        </Button>
                    </div>
                </div>
            )}
        </header>
    );
};

export default Header;
