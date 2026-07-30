import React from 'react';
import { useNavigate } from 'react-router-dom';
import { Button } from '@/components/ui/button';
import { Ghost } from 'lucide-react';

const NotFoundPage = () => {
    const navigate = useNavigate();

    return (
        <div className="flex min-h-screen flex-col items-center justify-center gap-4 bg-background p-4 text-center text-foreground">
            <Ghost className="h-16 w-16 text-muted-foreground" />
            <h1 className="font-display text-6xl font-black">404</h1>
            <p className="text-xl font-medium">Page not found</p>
            <p className="max-w-md text-muted-foreground">
                The page you&apos;re looking for might have been removed, renamed, or never existed.
            </p>
            {/* The one control on the page, and the only way out of it. The
                button scale's `default` is 36px tall — a pointer size. This is
                as likely to be hit by a thumb on a mistyped link as by a mouse,
                so it gets a real 44x44 target (WCAG 2.5.5) and keeps it at
                every width rather than relaxing above `sm`: there is nothing
                crowding it that a tighter size would buy room for. */}
            <Button className="h-11 min-w-[44px] px-6" onClick={() => navigate('/')}>
                Go home
            </Button>
        </div>
    );
};

export default NotFoundPage;
