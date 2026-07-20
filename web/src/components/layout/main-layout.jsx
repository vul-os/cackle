import React, { useCallback, useEffect, useRef, useState } from 'react';
import { Outlet } from 'react-router-dom';
import { useMediaQuery } from 'react-responsive';
import { MotionConfig } from 'framer-motion';
import { cn } from '@/lib/utils';
import SideNav from '../nav/side-nav';
import TopBar from '../nav/top-bar';
import PageTransition from '../motion/page-transition';

const TOP_BAR_HEIGHT = '4rem';
// Single source of truth for the expanded sidebar width. Both the desktop
// rail and the mobile drawer render at this width so the outer container
// and SideNav's own layout never disagree (a previous mismatch here — the
// wrapper fixed at w-16 while SideNav always rendered its w-60 expanded
// state on desktop — caused the main content pane to visually paint over
// and clip the nav labels down to their first couple of characters).
const SIDEBAR_WIDTH_CLASS = 'w-60';

const MainLayout = () => {
    const isMobile = useMediaQuery({ maxWidth: 640 });
    const [isExpanded, setIsExpanded] = useState(false);
    const sidenavRef = useRef(null);
    const toggleButtonRef = useRef(null);

    const handleDrawerToggle = useCallback(
        (event) => {
            if (isMobile) {
                event.stopPropagation();
                setIsExpanded((prev) => !prev);
            }
        },
        [isMobile],
    );

    useEffect(() => {
        const handleClick = (event) => {
            if (!isMobile) return;
            const clickedSidenav = sidenavRef.current?.contains(event.target);
            const clickedToggle = toggleButtonRef.current?.contains(event.target);
            if (!clickedSidenav && !clickedToggle) setIsExpanded(false);
        };
        document.addEventListener('click', handleClick);
        return () => document.removeEventListener('click', handleClick);
    }, [isMobile]);

    useEffect(() => {
        setIsExpanded(false);
    }, [isMobile]);

    return (
        <MotionConfig reducedMotion="user">
            <div className="flex h-screen flex-col bg-background text-foreground">
                <a
                    href="#main-content"
                    className="fixed left-2 top-2 z-[100] -translate-y-16 rounded-md bg-primary px-4 py-2 text-sm font-medium text-primary-foreground shadow-elevated transition-transform focus:translate-y-0"
                >
                    Skip to content
                </a>
                <TopBar onMenuClick={handleDrawerToggle} toggleButtonRef={toggleButtonRef} />

                <div className="flex flex-1 overflow-hidden" style={{ marginTop: TOP_BAR_HEIGHT }}>
                    {!isMobile && (
                        <aside
                            className={cn(
                                'h-full shrink-0 overflow-y-auto overflow-x-hidden border-r border-sidebar-border shadow-elevated',
                                SIDEBAR_WIDTH_CLASS,
                            )}
                        >
                            <SideNav isExpanded={true} isMobile={false} />
                        </aside>
                    )}

                    <main
                        id="main-content"
                        tabIndex={-1}
                        className="relative flex-grow overflow-y-auto bg-muted/30 p-4 outline-none sm:p-6 lg:p-8"
                    >
                        <div className="mx-auto max-w-[1600px]">
                            <PageTransition>
                                <Outlet />
                            </PageTransition>
                        </div>
                        {isMobile && isExpanded && (
                            <div className="fixed inset-0 z-10 animate-fade-in bg-black/50" onClick={() => setIsExpanded(false)} />
                        )}
                    </main>

                    {isMobile && (
                        <aside
                            ref={sidenavRef}
                            className={cn(
                                'fixed inset-y-0 left-0 z-20 h-full overflow-y-auto overflow-x-hidden shadow-floating transition-all duration-300 ease-emphasized',
                                isExpanded ? SIDEBAR_WIDTH_CLASS : 'w-0',
                            )}
                            style={{ top: TOP_BAR_HEIGHT }}
                        >
                            <SideNav isExpanded={isExpanded} isMobile={true} />
                        </aside>
                    )}
                </div>
            </div>
        </MotionConfig>
    );
};

export default MainLayout;
