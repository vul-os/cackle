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
            <Button onClick={() => navigate('/')}>Go Home</Button>
        </div>
    );
};

export default NotFoundPage;
