import React, { useCallback, useEffect, useRef, useState } from 'react';
import { Outlet } from 'react-router-dom';
import { useMediaQuery } from 'react-responsive';
import { cn } from '@/lib/utils';
import SideNav from '../nav/side-nav';
import TopBar from '../nav/top-bar';

const TOP_BAR_HEIGHT = '4rem';

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
        <div className="flex h-screen flex-col bg-background text-foreground">
            <TopBar onMenuClick={handleDrawerToggle} toggleButtonRef={toggleButtonRef} />

            <div className="flex flex-1 overflow-hidden" style={{ marginTop: TOP_BAR_HEIGHT }}>
                {!isMobile && (
                    <aside className="h-full w-16 shrink-0 overflow-y-auto shadow-lg">
                        <SideNav isExpanded={true} isMobile={false} />
                    </aside>
                )}

                <main className="relative flex-grow overflow-y-auto bg-muted/30 p-4 sm:p-6">
                    <Outlet />
                    {isMobile && isExpanded && (
                        <div className="fixed inset-0 z-10 bg-black/50" onClick={() => setIsExpanded(false)} />
                    )}
                </main>

                {isMobile && (
                    <aside
                        ref={sidenavRef}
                        className={cn(
                            'fixed inset-y-0 left-0 z-20 h-full overflow-y-auto shadow-lg transition-all duration-300 ease-in-out',
                            isExpanded ? 'w-60' : 'w-0',
                        )}
                        style={{ top: TOP_BAR_HEIGHT }}
                    >
                        <SideNav isExpanded={isExpanded} isMobile={true} />
                    </aside>
                )}
            </div>
        </div>
    );
};

export default MainLayout;
