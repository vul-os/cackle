import { format } from 'date-fns';
import { Calendar as CalendarIcon } from 'lucide-react';
import type { DateRange } from 'react-day-picker';
import { Button } from '@/components/ui/button';
import { Calendar } from '@/components/ui/calendar';
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover';
import { cn } from '@/lib/utils';

interface DatePickerWithRangeProps {
    date?: DateRange;
    setDate: (date: DateRange | undefined) => void;
    className?: string;
}

const DatePickerWithRange = ({ date, setDate, className }: DatePickerWithRangeProps) => {
    return (
        <div className={cn('grid gap-2', className)}>
            <Popover>
                <PopoverTrigger asChild>
                    <Button
                        id="date"
                        variant="outline"
                        className={cn('w-full justify-start text-left font-normal sm:w-[300px]', !date?.from && 'text-muted-foreground')}
                    >
                        <CalendarIcon className="mr-2 h-4 w-4" />
                        {date?.from ? (
                            date.to ? (
                                <>
                                    {format(date.from, 'LLL dd, y HH:mm')} - {format(date.to, 'LLL dd, y HH:mm')}
                                </>
                            ) : (
                                format(date.from, 'LLL dd, y HH:mm')
                            )
                        ) : (
                            <span>Pick a date range</span>
                        )}
                    </Button>
                </PopoverTrigger>
                <PopoverContent className="w-auto p-0" align="start">
                    <Calendar initialFocus mode="range" defaultMonth={date?.from} selected={date} onSelect={setDate} numberOfMonths={2} />
                </PopoverContent>
            </Popover>
        </div>
    );
};

export default DatePickerWithRange;
