import React from 'react';
import { Button } from '@/components/ui/button';
import { Printer, Eye, PrinterIcon } from 'lucide-react';
import { Link } from 'react-router-dom';

export const PrintTicketButtons = ({ ticketId, onPrint, isPrinting }) => {
    return (
        <div className="mt-4 flex gap-2 print:hidden">
            <Button onClick={() => onPrint(ticketId)} variant="outline" size="sm" disabled={isPrinting}>
                <Printer className="mr-2 h-4 w-4" />
                {isPrinting ? 'Printing...' : 'Print Ticket'}
            </Button>
            <Button variant="outline" size="sm" asChild>
                <Link to={`/ticket/${ticketId}`}>
                    <Eye className="mr-2 h-4 w-4" />
                    View Details
                </Link>
            </Button>
        </div>
    );
};

export const PrintAllButton = ({ onPrintAll, isPrinting, ticketsCount }) => {
    if (ticketsCount === 0) return null;

    return (
        <Button onClick={onPrintAll} variant="outline" size="sm" className="print:hidden" disabled={isPrinting}>
            <PrinterIcon className="mr-2 h-4 w-4" />
            {isPrinting ? 'Printing...' : 'Print All Tickets'}
        </Button>
    );
};
