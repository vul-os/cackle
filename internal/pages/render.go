package pages

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
	"time"

	"github.com/vul-os/cackle/internal/money"
)

//go:embed page.html
var templateFS embed.FS

// pageTemplate is parsed once at init. Parsing at init rather than per request
// means a template that would fail to parse takes the process down at boot,
// not a visitor's page load.
// pageTemplate deliberately registers NO template functions. Every decision
// the page makes is made in Go (see newView) and reaches the template as a
// plain field, so there is no helper here that could be given a value the
// escaper then trusts.
var pageTemplate = template.Must(template.New("page.html").ParseFS(templateFS, "page.html"))

// Event is the subset of an event a page renders. It is declared here, rather
// than imported from internal/events, so this package stays free of the domain
// layer — see the package doc's note on purity. internal/httpapi maps one to
// the other.
type Event struct {
	Slug        string
	Title       string
	Summary     string
	Description string
	VenueName   string
	Address     string
	StartsAt    time.Time
	EndsAt      time.Time
	Timezone    string
	// Status is the event's lifecycle status. "cancelled" is rendered as a
	// prominent notice: a host must not be able to hide, by simply omitting
	// a block, the single fact a visitor most needs.
	Status string
	// Currency is the event's own ISO-4217 code. There is no default and no
	// privileged currency; amounts are formatted with its own exponent.
	Currency string
}

// TicketType is one purchasable (or free) admission class as the page shows it.
type TicketType struct {
	ID          string
	Name        string
	Description string
	// PriceMinor is an integer count of the currency's minor unit. Zero is a
	// first-class case: a free RSVP event is an event.
	PriceMinor int64
	SoldOut    bool
}

// Image is one image from the event's OWN gallery. Only ids present in
// RenderInput.AllowedImages can appear in the output — see renderableImage.
type Image struct {
	ID     string
	Width  int
	Height int
}

// RenderInput is everything Render needs. Every field is supplied by the
// caller; Render performs no lookups, reads no clock and touches no network.
type RenderInput struct {
	Doc     Document
	Event   Event
	Tickets []TicketType
	// AllowedImages is the event's own gallery. It is the ONLY source of
	// images the rendered page may reference; an image block naming anything
	// else is dropped rather than rendered.
	AllowedImages []Image
	// BuyHref is where a tickets block's call to action points when the
	// document does not override it.
	BuyHref string
	// Nonce is a fresh, per-response CSP nonce for the single inline
	// <style> element. Render does not generate it: the same value has to
	// appear in the Content-Security-Policy header, and only the caller
	// writing that header can guarantee they match.
	Nonce string
}

// Render turns a validated Document into a complete HTML document.
//
// Every host value passes through html/template's contextual escaper in its
// real context. There is no template.HTML/CSS/URL/JS cast in this package, so
// there is no path by which a host value reaches the output unescaped. A
// colour that somehow got past Validate would be rewritten to ZgotmplZ by the
// CSS escaper rather than closing the <style> element; an href with a scheme
// Validate missed would be rewritten to "#ZgotmplZ" rather than becoming a
// javascript: link. Neither should ever happen — that is the point of having
// both layers.
func Render(in RenderInput) ([]byte, error) {
	if err := in.Doc.Validate(); err != nil {
		return nil, err
	}
	v, err := newView(in)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := pageTemplate.Execute(&buf, v); err != nil {
		return nil, fmt.Errorf("pages: render: %w", err)
	}
	return buf.Bytes(), nil
}

// view is the flattened, render-ready shape the template walks. Building it up
// front means the template contains no logic beyond ranging and branching on
// booleans — templates are the worst place to put a security-relevant decision,
// so none of them are in there.
type view struct {
	Lang      string
	Dir       string
	FontClass string
	CornClass string
	Nonce     string
	Theme     Theme
	Labels    Labels

	Title     string
	Summary   string
	Cancelled bool

	When  timeRange
	Where whereView

	Blocks []blockView
}

type timeRange struct {
	StartISO  string
	EndISO    string
	StartText string
	EndText   string
	ZoneText  string
	Known     bool
}

type whereView struct {
	VenueName string
	Address   string
	Known     bool
}

type blockView struct {
	Kind string // "heading" | "text" | "image" | "links" | "faq" | "tickets" | "details" | "divider"

	Level      int
	Text       string
	Paragraphs []string

	Image Image
	Alt   string

	Links []Link
	Items []FAQItem

	Tickets []ticketView
	CTAHref string

	When  timeRange
	Where whereView
}

type ticketView struct {
	Name        string
	Description string
	// Price is the amount already formatted in the event's own currency
	// using that currency's exponent (internal/money) — never a float, and
	// never a hardcoded two decimal places.
	Price    string
	Currency string
	Free     bool
	SoldOut  bool
}

func newView(in RenderInput) (*view, error) {
	labels := in.Doc.Labels.withDefaults()

	when := formatRange(in.Event.StartsAt, in.Event.EndsAt, in.Event.Timezone)
	where := whereView{
		VenueName: in.Event.VenueName,
		Address:   in.Event.Address,
		Known:     in.Event.VenueName != "" || in.Event.Address != "",
	}

	v := &view{
		Lang:      in.Doc.Lang,
		Dir:       dirOrDefault(in.Doc.Theme.Direction),
		FontClass: "font-" + valueOr(in.Doc.Theme.Font, FontSystem),
		CornClass: "corn-" + valueOr(in.Doc.Theme.Corners, CornersSoft),
		Nonce:     in.Nonce,
		Theme:     in.Doc.Theme,
		Labels:    labels,
		Title:     in.Event.Title,
		Summary:   in.Event.Summary,
		Cancelled: in.Event.Status == "cancelled",
		When:      when,
		Where:     where,
	}

	allowed := make(map[string]Image, len(in.AllowedImages))
	for _, img := range in.AllowedImages {
		allowed[img.ID] = img
	}

	tickets, err := ticketViews(in.Tickets, in.Event.Currency, labels)
	if err != nil {
		return nil, err
	}

	for _, b := range in.Doc.Blocks {
		switch b.Type {
		case BlockHeading:
			level := b.Level
			if level == 0 {
				level = 2
			}
			v.Blocks = append(v.Blocks, blockView{Kind: BlockHeading, Level: level, Text: b.Text})
		case BlockText:
			v.Blocks = append(v.Blocks, blockView{Kind: BlockText, Paragraphs: b.Paragraphs})
		case BlockImage:
			img, ok := allowed[b.ImageID]
			if !ok {
				// The id names an image this event does not own, or one
				// that has since been deleted. Dropping the block is the
				// only safe outcome: rendering it would emit a /media/{id}
				// URL for someone else's image, which is precisely the
				// cross-tenant leak this check exists to stop. Validate
				// cannot catch it — it has no database — so the render path
				// enforces it independently of the store-time check.
				continue
			}
			v.Blocks = append(v.Blocks, blockView{Kind: BlockImage, Image: img, Alt: b.Alt})
		case BlockLinks:
			v.Blocks = append(v.Blocks, blockView{Kind: BlockLinks, Links: b.Links})
		case BlockFAQ:
			v.Blocks = append(v.Blocks, blockView{Kind: BlockFAQ, Items: b.Items})
		case BlockTickets:
			href := b.CTAHref
			if href == "" {
				href = in.BuyHref
			}
			v.Blocks = append(v.Blocks, blockView{Kind: BlockTickets, Tickets: tickets, CTAHref: href})
		case BlockDetails:
			v.Blocks = append(v.Blocks, blockView{Kind: BlockDetails, When: when, Where: where})
		case BlockDivider:
			v.Blocks = append(v.Blocks, blockView{Kind: BlockDivider})
		}
	}
	return v, nil
}

func ticketViews(tts []TicketType, currency string, labels Labels) ([]ticketView, error) {
	out := make([]ticketView, 0, len(tts))
	for _, tt := range tts {
		tv := ticketView{
			Name:        tt.Name,
			Description: tt.Description,
			Currency:    currency,
			SoldOut:     tt.SoldOut,
		}
		if tt.PriceMinor == 0 {
			tv.Free = true
			tv.Price = labels.Free
		} else {
			amt, err := money.New(tt.PriceMinor, currency)
			if err != nil {
				return nil, fmt.Errorf("pages: render ticket price: %w", err)
			}
			major, err := amt.Major()
			if err != nil {
				return nil, fmt.Errorf("pages: render ticket price: %w", err)
			}
			tv.Price = major
		}
		out = append(out, tv)
	}
	return out, nil
}

// withDefaults fills in Cackle's own chrome strings for any label the host did
// not override.
func (l Labels) withDefaults() Labels {
	return Labels{
		Tickets:    valueOr(l.Tickets, "Tickets"),
		GetTickets: valueOr(l.GetTickets, "Get tickets"),
		Free:       valueOr(l.Free, "Free"),
		SoldOut:    valueOr(l.SoldOut, "Sold out"),
		When:       valueOr(l.When, "When"),
		Where:      valueOr(l.Where, "Where"),
		Cancelled:  valueOr(l.Cancelled, "This event has been cancelled."),
	}
}

func valueOr(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

func dirOrDefault(d string) string {
	if d == "" {
		return DirAuto
	}
	return d
}

// formatRange renders an event's start/end without naming a month, a weekday
// or an AM/PM marker in any language: ISO-8601-shaped local time plus the
// numeric UTC offset, with the machine-readable RFC-3339 value carried in the
// <time datetime> attribute for anything that wants to reformat it properly.
//
// The alternative — "Friday 14 August 2026, 7:30 PM" — would hardcode English
// into every host's page regardless of the event's own language, which is the
// same class of built-in assumption as a hardcoded currency symbol.
//
// The event's own IANA timezone is used when the runtime can resolve it. A
// binary built without an embedded tzdata (Cackle does not embed one) on a
// host with no zoneinfo falls back to UTC and SAYS SO in the rendered offset,
// rather than quietly presenting UTC as local time.
func formatRange(start, end time.Time, tz string) timeRange {
	if start.IsZero() {
		return timeRange{}
	}
	loc := time.UTC
	if tz != "" {
		if l, err := time.LoadLocation(tz); err == nil {
			loc = l
		}
	}
	s := start.In(loc)
	out := timeRange{
		Known:     true,
		StartISO:  s.Format(time.RFC3339),
		StartText: s.Format("2006-01-02 15:04"),
		ZoneText:  "UTC" + s.Format("-07:00"),
	}
	if !end.IsZero() {
		e := end.In(loc)
		out.EndISO = e.Format(time.RFC3339)
		out.EndText = e.Format("2006-01-02 15:04")
	}
	return out
}
