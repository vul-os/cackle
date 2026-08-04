import React, { useEffect, useState } from 'react';
import { useNavigate, useLocation, Link } from 'react-router-dom';
import { Button } from '@/components/ui/button';
import { Menu, X, Moon, Sun, LogOut, ShieldCheck, Package, Ticket } from 'lucide-react';
import { useTheme } from '@/components/theme-provider';
import CartDropdown from '@/pages/visitor/cart/dropdown';
import { useAuth } from '@/context/use-auth';
import { BrandLockup } from '@/components/brand/wordmark';

// Every control in this bar is a phone tap target before it is anything
// else, so the icon-only ones are held to 44×44 below `sm` (WCAG 2.5.5 and
// the platform HIG minimum alike) and allowed to relax to the button
// scale's own 36px once there is a pointer to aim with.
const TAP = 'h-11 w-11 sm:h-9 sm:w-9';

export interface HeaderProps {
    className?: string;
}

const Header = ({ className = '' }: HeaderProps) => {
    const [isMobileMenuOpen, setIsMobileMenuOpen] = useState(false);
    const { theme, setTheme } = useTheme();
    const { user, loading, signOut } = useAuth();
    const navigate = useNavigate();
    const location = useLocation();

    // A menu left open across a route change is a panel covering a page the
    // visitor already asked for. Close it whenever the location changes,
    // including navigations this component didn't initiate (the back button,
    // a link inside the cart dropdown).
    useEffect(() => {
        setIsMobileMenuOpen(false);
    }, [location.pathname]);

    const handleSignOut = async () => {
        await signOut();
        navigate('/');
    };

    const handleNavigation = (path: string) => {
        navigate(path);
        setIsMobileMenuOpen(false);
    };

    const AuthButtons = ({ isMobile = false }: { isMobile?: boolean }) => {
        // While the session is still resolving, render nothing rather than a
        // flash of "Sign up" that turns into "Sign out" a moment later.
        if (loading) return null;

        const wide = isMobile ? 'w-full justify-start' : '';

        if (user) {
            return (
                <>
                    <Button variant="ghost" className={wide} onClick={() => handleNavigation('/orders')}>
                        <Package className="mr-2 h-4 w-4" aria-hidden="true" />
                        Orders
                    </Button>
                    <Button variant="ghost" className={wide} onClick={() => handleNavigation('/admin')}>
                        <ShieldCheck className="mr-2 h-4 w-4" aria-hidden="true" />
                        Organiser
                    </Button>
                    <Button variant="ghost" className={wide} onClick={handleSignOut}>
                        <LogOut className="mr-2 h-4 w-4" aria-hidden="true" />
                        Sign out
                    </Button>
                </>
            );
        }

        return (
            <>
                <Button variant="ghost" className={wide} onClick={() => handleNavigation('/login')}>
                    Log in
                </Button>
                <Button className={isMobile ? 'w-full' : ''} onClick={() => handleNavigation('/signup')}>
                    Sign up
                </Button>
            </>
        );
    };

    return (
        <header className={`fixed left-0 right-0 top-0 z-50 border-b border-border bg-background/90 backdrop-blur-md ${className}`}>
            {/* Keyboard users land here first; without it the only way past a
                nav this size is to tab through all of it on every page. */}
            <a
                href="#main"
                className="sr-only focus:not-sr-only focus:absolute focus:left-4 focus:top-3 focus:z-50 focus:rounded-md focus:bg-primary focus:px-4 focus:py-2 focus:text-sm focus:font-semibold focus:text-primary-foreground"
            >
                Skip to content
            </a>

            <div className="container mx-auto px-4">
                <div className="flex h-16 items-center justify-between gap-2">
                    {/* The one place this page draws the identity — tile plus
                        wordmark, from the shared component. Three files used
                        to each hand-roll this, which is exactly how the mark
                        drifted to a lowercase all-red word with no dot. */}
                    <Link to="/" className="flex h-11 shrink-0 items-center rounded-md" aria-label="Cackle — home">
                        <BrandLockup size="md" />
                    </Link>

                    <div className="hidden items-center gap-1 md:flex">
                        <Button variant="ghost" asChild>
                            <Link to="/events">
                                <Ticket className="mr-2 h-4 w-4" aria-hidden="true" />
                                What&apos;s on
                            </Link>
                        </Button>
                        <CartDropdown />
                        <AuthButtons />
                        <Button
                            onClick={() => setTheme(theme === 'dark' ? 'light' : 'dark')}
                            variant="ghost"
                            size="icon"
                            aria-label={theme === 'dark' ? 'Switch to light theme' : 'Switch to dark theme'}
                        >
                            {theme === 'dark' ? <Sun className="h-5 w-5" /> : <Moon className="h-5 w-5" />}
                        </Button>
                    </div>

                    <div className="flex items-center gap-1 md:hidden">
                        <CartDropdown />
                        <Button
                            variant="ghost"
                            size="icon"
                            className={TAP}
                            onClick={() => setIsMobileMenuOpen((open) => !open)}
                            aria-label={isMobileMenuOpen ? 'Close menu' : 'Open menu'}
                            aria-expanded={isMobileMenuOpen}
                            aria-controls="visitor-mobile-menu"
                        >
                            {isMobileMenuOpen ? <X className="h-6 w-6" /> : <Menu className="h-6 w-6" />}
                        </Button>
                    </div>
                </div>
            </div>

            {isMobileMenuOpen && (
                <div id="visitor-mobile-menu" className="animate-rise-in border-t border-border bg-background md:hidden">
                    <div className="container mx-auto flex flex-col gap-1 px-4 py-3">
                        <Button variant="ghost" className="w-full justify-start" onClick={() => handleNavigation('/events')}>
                            <Ticket className="mr-2 h-4 w-4" aria-hidden="true" />
                            What&apos;s on
                        </Button>
                        {!loading && user && (
                            <Button variant="ghost" className="w-full justify-start" onClick={() => handleNavigation('/tickets')}>
                                <Ticket className="mr-2 h-4 w-4" aria-hidden="true" />
                                My tickets
                            </Button>
                        )}
                        <AuthButtons isMobile />
                        <Button
                            onClick={() => setTheme(theme === 'dark' ? 'light' : 'dark')}
                            variant="ghost"
                            className="w-full justify-start"
                        >
                            {theme === 'dark' ? (
                                <Sun className="mr-2 h-4 w-4" aria-hidden="true" />
                            ) : (
                                <Moon className="mr-2 h-4 w-4" aria-hidden="true" />
                            )}
                            {theme === 'dark' ? 'Light theme' : 'Dark theme'}
                        </Button>
                    </div>
                </div>
            )}
        </header>
    );
};

export default Header;
