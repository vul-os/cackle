import React from 'react';
import { Search } from 'lucide-react';
import { Input } from '@/components/ui/input';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';

export default function TicketFilters({
    search,
    setSearch,
    selectedEvent,
    setSelectedEvent,
    selectedTicketType,
    setSelectedTicketType,
    selectedStatus,
    setSelectedStatus,
    selectedTime,
    setSelectedTime,
    events,
    ticketTypes,
}) {
    return (
        <div className="flex flex-col gap-3 sm:flex-row sm:flex-wrap sm:items-center">
            <div className="relative sm:min-w-[220px] sm:flex-1">
                <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" aria-hidden="true" />
                <Input
                    value={search}
                    onChange={(e) => setSearch(e.target.value)}
                    placeholder="Search by event or venue"
                    className="pl-9"
                    aria-label="Search your tickets"
                />
            </div>

            <Select value={selectedEvent} onValueChange={setSelectedEvent}>
                <SelectTrigger className="sm:w-48" aria-label="Filter by event">
                    <SelectValue placeholder="All events" />
                </SelectTrigger>
                <SelectContent>
                    <SelectItem value="all">All events</SelectItem>
                    {events.map((event) => (
                        <SelectItem key={event.id} value={event.id}>
                            {event.title}
                        </SelectItem>
                    ))}
                </SelectContent>
            </Select>

            <Select value={selectedTicketType} onValueChange={setSelectedTicketType}>
                <SelectTrigger className="sm:w-44" aria-label="Filter by ticket type">
                    <SelectValue placeholder="All ticket types" />
                </SelectTrigger>
                <SelectContent>
                    <SelectItem value="all">All ticket types</SelectItem>
                    {ticketTypes.map((type) => (
                        <SelectItem key={type.id} value={type.id}>
                            {type.name}
                        </SelectItem>
                    ))}
                </SelectContent>
            </Select>

            <Select value={selectedTime} onValueChange={setSelectedTime}>
                <SelectTrigger className="sm:w-36" aria-label="Filter by date">
                    <SelectValue placeholder="Any time" />
                </SelectTrigger>
                <SelectContent>
                    <SelectItem value="all">Any time</SelectItem>
                    <SelectItem value="upcoming">Upcoming</SelectItem>
                    <SelectItem value="past">Past</SelectItem>
                </SelectContent>
            </Select>

            <Select value={selectedStatus} onValueChange={setSelectedStatus}>
                <SelectTrigger className="sm:w-36" aria-label="Filter by status">
                    <SelectValue placeholder="Any status" />
                </SelectTrigger>
                <SelectContent>
                    <SelectItem value="all">Any status</SelectItem>
                    <SelectItem value="valid">Valid</SelectItem>
                    <SelectItem value="void">Void</SelectItem>
                    <SelectItem value="refunded">Refunded</SelectItem>
                </SelectContent>
            </Select>
        </div>
    );
}
