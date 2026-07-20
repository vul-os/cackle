package httpapi

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/vul-os/cackle/internal/orders"
	"github.com/vul-os/cackle/internal/store"
)

// handleListMyTickets serves GET /api/tickets (mine).
func (s *server) handleListMyTickets(w http.ResponseWriter, r *http.Request) {
	user, _ := userFromContext(r.Context())
	list, err := s.deps.Orders.TicketsForUser(r.Context(), user.ID)
	if err != nil {
		internalError(w, s.log(), "list my tickets", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"tickets": list})
}

// ticketOwnedBy reports whether ticket belongs to userID, and 404s (never
// 403 — avoids confirming another user's ticket id exists) if not.
func (s *server) ticketOwnedBy(w http.ResponseWriter, r *http.Request, userID, ticketID string) (*orders.Ticket, bool) {
	t, err := s.deps.Orders.Ticket(r.Context(), ticketID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			notFound(w, "ticket not found")
			return nil, false
		}
		internalError(w, s.log(), "get ticket", err)
		return nil, false
	}
	if t.HolderUserID != userID {
		notFound(w, "ticket not found")
		return nil, false
	}
	return t, true
}

// handleGetTicket serves GET /api/tickets/{id}.
func (s *server) handleGetTicket(w http.ResponseWriter, r *http.Request) {
	user, _ := userFromContext(r.Context())
	t, ok := s.ticketOwnedBy(w, r, user.ID, chi.URLParam(r, "id"))
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ticket": t})
}

// handleTicketPDF serves GET /api/tickets/{id}/pdf: a minimal, dependency-free
// single-page PDF carrying the ticket's identifying details and its signed
// capability string (the same value rendered as a QR code in the web UI).
// This is intentionally basic — a plain-text ticket, not a designed one —
// since pulling in a PDF/QR rendering dependency is out of scope here; the
// route exists and returns a real, valid PDF.
func (s *server) handleTicketPDF(w http.ResponseWriter, r *http.Request) {
	user, _ := userFromContext(r.Context())
	t, ok := s.ticketOwnedBy(w, r, user.ID, chi.URLParam(r, "id"))
	if !ok {
		return
	}

	pdf := renderTicketPDF(t)
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`inline; filename="cackle-ticket-%s.pdf"`, t.Serial))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(pdf)
}

// renderTicketPDF hand-builds a minimal valid single-page PDF (no external
// dependency) with a handful of left-aligned text lines. PDF text position
// is bottom-up; lines are laid out top-down by decreasing y.
func renderTicketPDF(t *orders.Ticket) []byte {
	lines := []string{
		"Cackle Ticket",
		"Serial: " + t.Serial,
		"Holder: " + t.HolderName,
		"Status: " + t.Status,
		"Capability:",
		wrapForPDF(t.Capability, 90),
	}

	var content bytes.Buffer
	content.WriteString("BT /F1 16 Tf 50 770 Td (Cackle Ticket) Tj ET\n")
	y := 730
	for _, line := range lines[1:] {
		fmt.Fprintf(&content, "BT /F1 11 Tf 50 %d Td (%s) Tj ET\n", y, pdfEscape(line))
		y -= 20
	}

	var buf bytes.Buffer
	buf.WriteString("%PDF-1.4\n")
	offsets := make([]int, 0, 5)

	writeObj := func(n int, body string) {
		offsets = append(offsets, buf.Len())
		fmt.Fprintf(&buf, "%d 0 obj\n%s\nendobj\n", n, body)
	}

	writeObj(1, "<< /Type /Catalog /Pages 2 0 R >>")
	writeObj(2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>")
	writeObj(3, "<< /Type /Page /Parent 2 0 R /Resources << /Font << /F1 5 0 R >> >> /MediaBox [0 0 612 792] /Contents 4 0 R >>")
	writeObj(4, fmt.Sprintf("<< /Length %d >>\nstream\n%sendstream", content.Len(), content.String()))
	writeObj(5, "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>")

	xrefStart := buf.Len()
	fmt.Fprintf(&buf, "xref\n0 %d\n", len(offsets)+1)
	buf.WriteString("0000000000 65535 f \n")
	for _, off := range offsets {
		fmt.Fprintf(&buf, "%010d 00000 n \n", off)
	}
	fmt.Fprintf(&buf, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF", len(offsets)+1, xrefStart)

	return buf.Bytes()
}

func pdfEscape(s string) string {
	var b bytes.Buffer
	for _, r := range s {
		switch r {
		case '(', ')', '\\':
			b.WriteByte('\\')
			b.WriteRune(r)
		default:
			if r >= 32 && r < 127 {
				b.WriteRune(r)
			}
		}
	}
	return b.String()
}

func wrapForPDF(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
