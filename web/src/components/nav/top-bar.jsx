import React from 'react';
import { Link } from 'react-router-dom';
import { Menu, User, ChevronDown, Building2, LogOut, Moon, Sun } from 'lucide-react';
import { useAuth } from '@/context/use-auth';
import { useTheme } from '@/components/theme-provider';
import { Button } from '@/components/ui/button';
import {
    DropdownMenu,
    DropdownMenuContent,
    DropdownMenuItem,
    DropdownMenuLabel,
    DropdownMenuSeparator,
    DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { BrandLockup } from '@/components/brand/wordmark';

const TopBar = ({ onMenuClick, toggleButtonRef }) => {
    const { user, signOut, orgs, activeOrg, switchOrg } = useAuth();
    const { theme, setTheme } = useTheme();

    return (
        <nav className="fixed left-0 right-0 top-0 z-50 flex h-16 items-center justify-between border-b border-sidebar-border bg-sidebar px-4 text-sidebar-foreground shadow-elevated sm:px-6">
            <div className="flex items-center gap-3">
                {/* This is the ONLY route to the side nav on a phone, on all
                    14 console pages, and gate staff work the door on a phone.
                    It measured 22×22 — under WCAG 2.2 AA's 24×24 floor and
                    less than a quarter of the 44×44 target the platform HIGs
                    ask for. The hit area is now 44×44 while the glyph stays
                    at 22, so nothing about the chrome looks heavier; the
                    negative margin keeps the icon optically flush with the
                    wordmark rather than pushed in by its own padding. */}
                <button
                    ref={toggleButtonRef}
                    className="-ml-2.5 inline-flex h-11 w-11 shrink-0 items-center justify-center rounded-md text-sidebar-foreground/80 transition-colors hover:bg-sidebar-accent hover:text-sidebar-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-sidebar-background sm:hidden"
                    onClick={onMenuClick}
                    aria-label="Open navigation menu"
                >
                    <Menu size={22} aria-hidden="true" />
                </button>
                {/* The console chrome is a fixed ink shell in both themes, so
                    the wordmark is pinned to its on-dark tone rather than
                    following the page. Below sm only the tile draws — at
                    390px the org switcher and the account menu need the
                    width more than the word does. */}
                <Link
                    to="/"
                    aria-label="Cackle home"
                    className="rounded-md transition-opacity hover:opacity-90 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-sidebar-background"
                >
                    <BrandLockup size="md" tone="onDark" hideWordBelowSm />
                </Link>
            </div>

            <div className="flex items-center gap-2">
                {orgs?.length > 1 && (
                    <DropdownMenu>
                        <DropdownMenuTrigger asChild>
                            <Button
                                variant="ghost"
                                className="gap-2 text-sidebar-foreground/80 hover:bg-sidebar-accent hover:text-sidebar-accent-foreground"
                            >
                                <Building2 size={18} />
                                <span className="max-w-[140px] truncate">{activeOrg?.name || 'Select org'}</span>
                                <ChevronDown size={14} />
                            </Button>
                        </DropdownMenuTrigger>
                        <DropdownMenuContent align="end">
                            <DropdownMenuLabel>Organizations</DropdownMenuLabel>
                            <DropdownMenuSeparator />
                            {orgs.map((org) => (
                                <DropdownMenuItem key={org.id} onClick={() => switchOrg(org.id)}>
                                    <Building2 className="mr-2 h-4 w-4 text-muted-foreground" />
                                    {org.name}
                                </DropdownMenuItem>
                            ))}
                        </DropdownMenuContent>
                    </DropdownMenu>
                )}

                <Button
                    variant="ghost"
                    size="icon"
                    className="text-sidebar-foreground/80 hover:bg-sidebar-accent hover:text-sidebar-accent-foreground"
                    onClick={() => setTheme(theme === 'dark' ? 'light' : 'dark')}
                    aria-label="Toggle theme"
                >
                    {theme === 'dark' ? <Sun size={18} /> : <Moon size={18} />}
                </Button>

                {user && (
                    <DropdownMenu>
                        <DropdownMenuTrigger asChild>
                            <Button
                                variant="ghost"
                                size="icon"
                                className="text-sidebar-foreground/80 hover:bg-sidebar-accent hover:text-sidebar-accent-foreground"
                                aria-label="Account menu"
                            >
                                <User size={20} />
                            </Button>
                        </DropdownMenuTrigger>
                        <DropdownMenuContent align="end">
                            <DropdownMenuLabel className="truncate">{user.email}</DropdownMenuLabel>
                            <DropdownMenuSeparator />
                            <DropdownMenuItem asChild>
                                <Link to="/">Visitor site</Link>
                            </DropdownMenuItem>
                            <DropdownMenuItem onClick={signOut} className="text-destructive focus:text-destructive">
                                <LogOut className="mr-2 h-4 w-4" />
                                Sign out
                            </DropdownMenuItem>
                        </DropdownMenuContent>
                    </DropdownMenu>
                )}
            </div>
        </nav>
    );
};

export default TopBar;
