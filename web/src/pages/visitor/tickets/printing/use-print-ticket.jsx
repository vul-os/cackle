// hooks/use-print-ticket.js
import { useState } from 'react';

export const usePrintTicket = () => {
  const [isPrinting, setIsPrinting] = useState(false);

  const getTicketHTML = (ticketElements) => `
    <!DOCTYPE html>
    <html>
      <head>
        <title>Print Tickets</title>
        <style>
          @page {
            size: A4;
            margin: 0;
          }

          html, body {
            margin: 0;
            padding: 0;
            background: white;
            font-family: system-ui, -apple-system, sans-serif;
          }

          .page {
            width: 210mm;
            min-height: 297mm;
            padding: 15mm;
            margin: 0 auto;
            background: white;
            box-sizing: border-box;
            display: flex;
            flex-direction: column;
            gap: 20mm; /* More space between tickets */
          }

          .ticket-wrapper {
            position: relative;
            width: 180mm;
            height: 72mm; /* Slightly taller for better content fit */
            margin: 0 auto;
            page-break-inside: avoid;
            background: white;
          }

          /* Cutting guides */
          .cutting-guide {
            position: absolute;
            top: -3mm;
            left: -3mm;
            right: -3mm;
            bottom: -3mm;
            border: 1px dashed #CBD5E1;
            pointer-events: none;
          }

          /* Corner marks */
          .corner-mark {
            position: absolute;
            width: 5mm;
            height: 5mm;
          }

          .corner-mark::before,
          .corner-mark::after {
            content: '';
            position: absolute;
            background-color: #CBD5E1;
          }

          .corner-mark.top-left {
            top: -3mm;
            left: -3mm;
          }

          .corner-mark.top-right {
            top: -3mm;
            right: -3mm;
          }

          .corner-mark.bottom-left {
            bottom: -3mm;
            left: -3mm;
          }

          .corner-mark.bottom-right {
            bottom: -3mm;
            right: -3mm;
          }

          .corner-mark::before {
            width: 5mm;
            height: 1px;
            top: 50%;
          }

          .corner-mark::after {
            width: 1px;
            height: 5mm;
            left: 50%;
          }

          /* Scissors icons */
          .scissors {
            position: absolute;
            font-size: 14px;
            color: #9CA3AF;
          }

          .scissors.top {
            top: -6mm;
            left: 50%;
            transform: translateX(-50%);
          }

          .scissors.bottom {
            bottom: -6mm;
            left: 50%;
            transform: translateX(-50%);
          }

          .scissors.left {
            left: -6mm;
            top: 50%;
            transform: translateY(-50%) rotate(-90deg);
          }

          .scissors.right {
            right: -6mm;
            top: 50%;
            transform: translateY(-50%) rotate(90deg);
          }

          /* Ticket container */
          .ticket-container {
            width: 100%;
            height: 100%;
            overflow: hidden;
          }

          /* Ticket styles */
          .printable-ticket {
            width: 100% !important;
            height: 100% !important;
            background: white !important;
            border: 1px solid #E5E7EB !important;
            border-radius: 4px !important;
            overflow: hidden !important;
            box-sizing: border-box !important;
            display: flex !important;
            flex-direction: column !important;
          }

          .printable-ticket > div {
            height: 100% !important;
          }

          /* Utility classes */
          .flex { display: flex !important; }
          .flex-[3] { flex: 3 !important; }
          .flex-1 { flex: 1 !important; }
          .p-6, .p-8 { padding: 16px !important; }
          .pr-8 { padding-right: 24px !important; }
          .pl-8 { padding-left: 24px !important; }
          .mb-6 { margin-bottom: 1.5rem !important; }
          .mb-1 { margin-bottom: 0.25rem !important; }
          .mt-4 { margin-top: 1rem !important; }
          .items-center { align-items: center !important; }
          .items-start { align-items: flex-start !important; }
          .justify-center { justify-content: center !important; }
          .justify-between { justify-content: space-between !important; }
          .space-y-4 > * + * { margin-top: 1rem !important; }
          .space-x-3 > * + * { margin-left: 0.75rem !important; }
          .border-r { border-right: 2px dashed #CBD5E1 !important; }
          .border-t { border-top: 1px dashed #E5E7EB !important; }
          
          .text-3xl { 
            font-size: 1.875rem !important;
            line-height: 2.25rem !important;
          }
          .text-sm { font-size: 0.875rem !important; }
          .text-xs { font-size: 0.75rem !important; }
          
          .font-bold { font-weight: 700 !important; }
          .font-medium { font-weight: 500 !important; }
          .font-mono { font-family: ui-monospace, monospace !important; }
          
          .text-black { color: black !important; }
          .text-gray-500 { color: #6B7280 !important; }
          .text-gray-600 { color: #4B5563 !important; }
          .text-gray-700 { color: #374151 !important; }
          .text-primary { color: #e11d3f !important; }
          
          .bg-white { background-color: white !important; }
          .print\\:hidden { display: none !important; }
          .rounded-xl { border-radius: 0.75rem !important; }

          .grid {
            display: grid !important;
            grid-template-columns: repeat(2, 1fr) !important;
            gap: 2rem !important;
          }

          /* QR Code container */
          .bg-white.p-3.rounded-xl {
            padding: 0.75rem !important;
            background: white !important;
            box-shadow: 0 1px 3px 0 rgba(0, 0, 0, 0.1) !important;
          }

          /* QR Code size */
          .bg-white.p-3.rounded-xl svg {
            width: 100px !important;
            height: 100px !important;
          }

          @media print {
            .page {
              margin: 0;
              page-break-after: always;
            }

            .page:last-child {
              page-break-after: avoid;
            }

            .ticket-wrapper {
              break-inside: avoid;
            }

            /* Print colors */
            .cutting-guide {
              border-color: #000 !important;
            }

            .corner-mark::before,
            .corner-mark::after {
              background-color: #000 !important;
            }

            .scissors {
              color: #000 !important;
            }
          }
        </style>
      </head>
      <body>
        ${typeof ticketElements === 'string' 
          ? `<div class="page">${wrapTicket(ticketElements)}</div>`
          : ticketElements.reduce((pages, ticket, index) => {
              if (index % 3 === 0) pages.push([]);
              pages[pages.length - 1].push(wrapTicket(ticket));
              return pages;
            }, [])
            .map(pageTickets => `
              <div class="page">
                ${pageTickets.join('')}
              </div>
            `).join('')
        }
      </body>
    </html>
  `;

  const wrapTicket = (ticketHtml) => `
    <div class="ticket-wrapper">
      <div class="cutting-guide"></div>
      <div class="corner-mark top-left"></div>
      <div class="corner-mark top-right"></div>
      <div class="corner-mark bottom-left"></div>
      <div class="corner-mark bottom-right"></div>
      <div class="scissors top">✂</div>
      <div class="scissors bottom">✂</div>
      <div class="scissors left">✂</div>
      <div class="scissors right">✂</div>
      <div class="ticket-container">
        ${ticketHtml}
      </div>
    </div>
  `;

  const printSingleTicket = (ticketId) => {
    if (isPrinting) return;
    setIsPrinting(true);
    
    const ticketElement = document.getElementById(`printable-ticket-${ticketId}`);
    
    if (ticketElement) {
      const printWindow = window.open('', '_blank', 'width=800,height=600');
      printWindow.document.write(getTicketHTML(ticketElement.outerHTML));
      printWindow.document.close();
    
      setTimeout(() => {
        printWindow.focus();
        printWindow.print();
        setTimeout(() => {
          printWindow.close();
          setIsPrinting(false);
        }, 500);
      }, 500);
    } else {
      setIsPrinting(false);
    }
  };

  const printAllTickets = (filteredTickets) => {
    if (isPrinting) return;
    setIsPrinting(true);

    const printWindow = window.open('', '', 'width=800,height=600');
    const ticketElements = filteredTickets.map(ticket => 
      document.getElementById(`printable-ticket-${ticket.id}`).outerHTML
    );

    printWindow.document.write(getTicketHTML(ticketElements));
    printWindow.document.close();
    printWindow.focus();
    
    setTimeout(() => {
      printWindow.print();
      setTimeout(() => {
        printWindow.close();
        setIsPrinting(false);
      }, 500);
    }, 250);
  };

  return {
    isPrinting,
    printSingleTicket,
    printAllTickets
  };
};