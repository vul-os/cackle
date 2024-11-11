import React from 'react';
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from '@/components/ui/card';

export default function EventInformation({ event }) {
  if (!event.description && !event.information && !event.policy_info) {
    return null;
  }

  return (
    <Card className="mt-4 print:hidden">
      <CardHeader>
        <CardTitle className="dark:text-white">{event.title} - Event Information</CardTitle>
      </CardHeader>
      <CardContent className="space-y-6">
        {event.description && (
          <div>
            <h3 className="font-semibold mb-2 dark:text-white">Description</h3>
            <p className="text-gray-600 dark:text-gray-300 whitespace-pre-wrap">{event.description}</p>
          </div>
        )}

        {event.information && (
          <div>
            <h3 className="font-semibold mb-2 dark:text-white">Additional Information</h3>
            <div className="prose dark:prose-invert max-w-none">
              <div dangerouslySetInnerHTML={{ __html: event.information }} />
            </div>
          </div>
        )}

        {event.policy_info && (
          <div>
            <h3 className="font-semibold mb-2 dark:text-white">Event Policies</h3>
            <div className="prose dark:prose-invert max-w-none">
              <div dangerouslySetInnerHTML={{ __html: event.policy_info }} />
            </div>
          </div>
        )}
      </CardContent>
    </Card>
  );
}