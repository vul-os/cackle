import React from 'react';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';

export default function EventInformation({ event }) {
    if (!event.description && !event.summary) return null;

    return (
        <Card className="mt-4 print:hidden">
            <CardHeader>
                <CardTitle>{event.title} — Event Information</CardTitle>
            </CardHeader>
            <CardContent className="space-y-4">
                {event.summary && <p className="text-muted-foreground">{event.summary}</p>}
                {event.description && <p className="whitespace-pre-wrap text-muted-foreground">{event.description}</p>}
            </CardContent>
        </Card>
    );
}
