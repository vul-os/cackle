"use client";

import * as React from 'react';
import { Card, CardContent } from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import { Textarea } from '@/components/ui/textarea';
import { CategorySection } from './categories';
import { Calendar, MapPin, Link2, Globe } from 'lucide-react';
import DatePickerWithRange from '@/components/date-range-picker';

export const EventDetailsCard = ({
  editForm,
  handleInputChange,
  initialData,
  isSubmitting,
  categories,
  availableSubcategories
}) => {
  const [date, setDate] = React.useState(() => {
    if (editForm?.start_time && editForm?.end_time) {
      return {
        from: new Date(editForm.start_time),
        to: new Date(editForm.end_time)
      };
    }
    return undefined;
  });

  // Keep date in sync with editForm
  React.useEffect(() => {
    if (editForm?.start_time && editForm?.end_time) {
      setDate({
        from: new Date(editForm.start_time),
        to: new Date(editForm.end_time)
      });
    }
  }, [editForm?.start_time, editForm?.end_time]);

  // Watch for date changes and update form
  React.useEffect(() => {
    if (date?.from) {
      handleInputChange('start_time', date.from.toISOString());
    }
    if (date?.to) {
      handleInputChange('end_time', date.to.toISOString());
    }
  }, [date, handleInputChange]);

  const inputClasses = "border-gray-200 hover:border-gray-300 transition-colors bg-white";

  return (
    <Card className="shadow-lg border-gray-200/80">
      <CardContent className="space-y-8 pt-6">
        <div className="space-y-4">
          <div className="flex items-center gap-2 text-gray-500">
            <Calendar className="h-4 w-4" />
            <h2 className="text-sm font-medium">Date & Time</h2>
          </div>
          <div className="space-y-2">
            <label className="text-sm font-medium leading-none peer-disabled:cursor-not-allowed peer-disabled:opacity-70">
              Event Period
            </label>
            <div>
              <DatePickerWithRange 
                date={date}
                setDate={setDate}
                className="w-full"
              />
            </div>
            {!initialData && (
              <p className="text-sm text-muted-foreground mt-1">
                Please select both start and end dates
              </p>
            )}
          </div>
        </div>

        <div className="space-y-4 border-t pt-6">
          <div className="flex items-center gap-2 text-gray-500">
            <MapPin className="h-4 w-4" />
            <h2 className="text-sm font-medium">Location</h2>
          </div>
          <div className="space-y-4">
            <Input
              placeholder="Venue Name"
              value={editForm.venue_name}
              onChange={(e) => handleInputChange('venue_name', e.target.value)}
              className={`${inputClasses} font-medium`}
              disabled={isSubmitting}
            />
            <Input
              placeholder="Venue Address"
              value={editForm.venue_address}
              onChange={(e) => handleInputChange('venue_address', e.target.value)}
              className={`${inputClasses} text-gray-600`}
              disabled={isSubmitting}
            />
          </div>
        </div>

        {categories && (
          <CategorySection
            editForm={editForm}
            handleInputChange={handleInputChange}
            categories={categories}
            availableSubcategories={availableSubcategories}
            disabled={isSubmitting}
          />
        )}

        <div className="space-y-4 border-t pt-6">
          <div className="flex items-center gap-2 text-gray-500">
            <Globe className="h-4 w-4" />
            <h2 className="text-sm font-medium">Event URL</h2>
          </div>
          <Input
            placeholder="https://"
            value={editForm.url}
            onChange={(e) => handleInputChange('url', e.target.value)}
            className={`${inputClasses} text-blue-600`}
            disabled={isSubmitting}
          />
        </div>

        <div className="space-y-4 border-t pt-6">
          <div className="flex items-center gap-2 text-gray-500">
            <Link2 className="h-4 w-4" />
            <h2 className="text-sm font-medium">Description</h2>
          </div>
          <Textarea
            placeholder="Event description"
            value={editForm.description}
            onChange={(e) => handleInputChange('description', e.target.value)}
            className={`${inputClasses} min-h-[200px] resize-none`}
            disabled={isSubmitting}
          />
        </div>
      </CardContent>
    </Card>
  );
};

export default EventDetailsCard;