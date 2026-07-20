import React from 'react';
import { QRCodeSVG } from 'qrcode.react';
import { Calendar, MapPin, Tag, Scissors } from 'lucide-react';
import { formatDate, formatTime } from '../date-utils';

export default function TicketLayout({ ticket, event, type }) {
    return (
        <div className="p-8 print:m-0 print:p-0">
            <div
                id={`printable-ticket-${ticket.id}`}
                className="relative overflow-visible rounded-lg border border-dashed border-border bg-gradient-to-r from-muted/40 to-card shadow-lg print:border-gray-300 print:bg-white print:from-white print:to-white print:shadow-none"
            >
                <div className="absolute -left-4 -top-4 print:hidden">
                    <Scissors className="h-6 w-6 -rotate-45 text-muted-foreground" />
                </div>
                <div className="absolute -right-4 -top-4 print:hidden">
                    <Scissors className="h-6 w-6 rotate-45 text-muted-foreground" />
                </div>

                <div className="flex p-6 print:min-h-[2.75in] print:p-4">
                    <div className="flex-[3] border-r border-dashed border-border pr-8 print:border-gray-400">
                        <div className="mb-6 flex items-start justify-between">
                            <div>
                                <h2 className="mb-1 text-3xl font-bold print:text-black">{event.title}</h2>
                                <p className="text-sm font-medium text-primary print:text-gray-700">{type.name}</p>
                            </div>
                            <div className="font-mono text-sm text-muted-foreground print:text-gray-600">#{ticket.serial}</div>
                        </div>

                        <div className="mb-6 grid grid-cols-2 gap-8 print:gap-4">
                            <div className="space-y-4">
                                <div className="flex items-center space-x-3">
                                    <Calendar className="h-5 w-5 text-primary print:text-gray-700" />
                                    <div>
                                        <div className="text-sm font-medium print:text-black">{formatDate(event.starts_at)}</div>
                                        <div className="text-sm text-muted-foreground print:text-gray-600">{formatTime(event.starts_at)}</div>
                                    </div>
                                </div>
                                <div className="flex items-start space-x-3">
                                    <MapPin className="mt-0.5 h-5 w-5 text-primary print:text-gray-700" />
                                    <div>
                                        <div className="text-sm font-medium print:text-black">{event.venue_name}</div>
                                        <div className="text-sm text-muted-foreground print:text-gray-600">{event.address}</div>
                                    </div>
                                </div>
                            </div>
                            <div className="space-y-4">
                                <div className="flex items-center space-x-3">
                                    <Tag className="h-5 w-5 text-primary print:text-gray-700" />
                                    <div className="text-sm font-medium print:text-black">{type.name}</div>
                                </div>
                            </div>
                        </div>

                        <div className="border-t border-dashed border-border pt-4 print:border-gray-300">
                            <p className="text-xs text-muted-foreground print:text-gray-600">
                                Present this ticket at the entrance. Valid for one-time entry only.
                            </p>
                        </div>
                    </div>

                    <div className="flex flex-1 flex-col items-center justify-center pl-8 print:pl-4">
                        <div className="rounded-xl bg-white p-3 shadow-sm print:p-2 print:shadow-none">
                            {ticket.capability && <QRCodeSVG value={ticket.capability} size={120} level="H" />}
                        </div>
                        <div className="mt-4 text-center font-mono text-sm text-muted-foreground print:text-gray-700">{ticket.serial}</div>
                    </div>
                </div>
            </div>
        </div>
    );
}
