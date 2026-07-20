// Package demo seeds realistic South African events + ticketing data for
// `cackle --demo`, so the product is fully explorable with zero manual
// setup — the screenshotter and every first-time user depend on this
// working. Seed is idempotent: running it twice is a no-op the second
// time (detected by the presence of the demo org).
package demo

import (
	"context"
	"fmt"
	"time"

	"github.com/vul-os/cackle/internal/auth"
	"github.com/vul-os/cackle/internal/events"
	"github.com/vul-os/cackle/internal/orders"
	"github.com/vul-os/cackle/internal/payments"
	"github.com/vul-os/cackle/internal/store"
)

// DemoEmail and DemoPassword are the credentials printed by `cackle
// --demo` at boot and used to log into the seeded org. Exported so
// cmd/cackle can print exactly what Seed created without duplicating the
// literals.
const (
	DemoEmail    = "demo@cackle.events"
	DemoPassword = "demo1234"
	DemoOrgSlug  = "cackle-demo"
	DemoOrgName  = "Cackle Demo Events"
)

type seedEvent struct {
	slug        string
	title       string
	summary     string
	description string
	venueName   string
	address     string
	coverImage  string
	daysFromNow int
	durationHrs int
	ticketTypes []seedTicketType
}

type seedTicketType struct {
	name          string
	description   string
	priceCents    int64
	quantityTotal int
}

func seedEvents() []seedEvent {
	return []seedEvent{
		{
			slug: "rocking-the-daisies", title: "Rocking the Daisies",
			summary:     "Three days of music, wine, and dust on the West Coast.",
			description: "South Africa's original boutique music festival returns to Cloof Wine Estate with four stages, a silent disco, and the best sundowner spot in the Western Cape.",
			venueName:   "Cloof Wine Estate", address: "Darling, Western Cape",
			coverImage: "/images/festival.jpg", daysFromNow: 62, durationHrs: 72,
			ticketTypes: []seedTicketType{
				{"Early Bird 3-Day", "Full weekend, camping included.", 129900, 400},
				{"General 3-Day", "Full weekend, camping included.", 179900, 1200},
				{"VIP 3-Day", "Backstage viewing deck + fast-lane entry.", 349900, 150},
			},
		},
		{
			slug: "karoo-rock-revival", title: "Karoo Rock Revival",
			summary:     "A one-night rock lineup under the Karoo sky.",
			description: "Local and touring rock acts play the Herald amphitheatre for one loud, dusty night. Bring a blanket, leave the earplugs at home.",
			venueName:   "CP Nel Amphitheatre", address: "Oudtshoorn, Western Cape",
			coverImage: "/images/rockfest.jpg", daysFromNow: 34, durationHrs: 8,
			ticketTypes: []seedTicketType{
				{"General Admission", "Standing, all ages.", 39900, 800},
				{"VIP", "Front-of-stage pit + free drink voucher.", 89900, 100},
			},
		},
		{
			slug: "jozi-jazz-and-soul-night", title: "Jozi Jazz & Soul Night",
			summary:     "An intimate evening of jazz and soul in Braamfontein.",
			description: "A rotating lineup of Johannesburg's best jazz and soul acts, seated cabaret-style with a full bar.",
			venueName:   "The Bassline", address: "Braamfontein, Johannesburg",
			coverImage: "/images/music.jpg", daysFromNow: 18, durationHrs: 5,
			ticketTypes: []seedTicketType{
				{"Standard", "Seated, general area.", 24900, 250},
				{"Table for 4", "Reserved table, front section.", 129900, 20},
			},
		},
		{
			slug: "cape-town-pub-quiz-championship", title: "Cape Town Pub Quiz Championship",
			summary:     "Teams of up to 6 battle it out for the city title.",
			description: "Round after round of general knowledge, music, and local trivia. Prizes for the top three teams; last place gets a wooden spoon, literally.",
			venueName:   "Forries Bar", address: "Observatory, Cape Town",
			coverImage: "/images/quiz.jpg", daysFromNow: 9, durationHrs: 3,
			ticketTypes: []seedTicketType{
				{"Team Entry (up to 6)", "One ticket covers the whole team.", 30000, 60},
			},
		},
		{
			slug: "sunrise-yoga-retreat-hermanus", title: "Sunrise Yoga Retreat",
			summary:     "A weekend of yoga, whale watching, and quiet mornings.",
			description: "Two nights in Hermanus with twice-daily yoga sessions, plant-based meals, and time to just watch the ocean.",
			venueName:   "Aloe Ridge Retreat", address: "Hermanus, Western Cape",
			coverImage: "/images/yoga.jpg", daysFromNow: 45, durationHrs: 48,
			ticketTypes: []seedTicketType{
				{"Shared Room", "Twin-share accommodation, all meals included.", 349900, 24},
				{"Private Room", "Single room, all meals included.", 499900, 10},
			},
		},
		{
			slug: "karoo-100-trail-run", title: "Karoo 100 Trail Run",
			summary:     "100km, 50km, and 21km trail routes through the Camdeboo.",
			description: "A self-sufficient trail run across Karoo farmland with three distance options, water points every 10km, and a finish-line potjie.",
			venueName:   "Camdeboo National Park", address: "Graaff-Reinet, Eastern Cape",
			coverImage: "/images/racing.jpeg", daysFromNow: 76, durationHrs: 14,
			ticketTypes: []seedTicketType{
				{"21km Entry", "Timed, includes finisher medal.", 45000, 300},
				{"50km Entry", "Timed, includes finisher medal + meal.", 65000, 200},
				{"100km Entry", "Timed, includes finisher medal + meal + t-shirt.", 95000, 100},
			},
		},
		{
			slug: "origin-coffee-festival", title: "Origin Coffee Festival",
			summary:     "Roasters, baristas, and cupping sessions all weekend.",
			description: "Independent roasters from across the country pour flights, run latte-art jams, and host cupping workshops for beginners and obsessives alike.",
			venueName:   "The Old Biscuit Mill", address: "Woodstock, Cape Town",
			coverImage: "/images/coffee.jpg", daysFromNow: 23, durationHrs: 16,
			ticketTypes: []seedTicketType{
				{"Day Pass", "Entry + 5 tasting tokens.", 18000, 500},
				{"Weekend Pass", "Entry both days + 12 tasting tokens.", 30000, 200},
			},
		},
		{
			slug: "durban-beachfront-fun-run", title: "Durban Beachfront Fun Run",
			summary:     "A flat, fast 10km along the Golden Mile.",
			description: "An early-start, family-friendly 10km and 5km along Durban's beachfront promenade, finishing with a breakfast market.",
			venueName:   "North Beach", address: "Durban, KwaZulu-Natal",
			coverImage: "/images/jog.jpg", daysFromNow: 12, durationHrs: 4,
			ticketTypes: []seedTicketType{
				{"5km Entry", "Timed, includes race number.", 15000, 600},
				{"10km Entry", "Timed, includes race number + t-shirt.", 25000, 600},
			},
		},
	}
}

// Seed populates a demo org, a demo user, ~8 published events with ticket
// types, a handful of paid orders with issued tickets, and a few
// admissions so the scanner and stats screens look alive. It is safe to
// call on every `--demo` boot: if the demo org already exists, Seed
// returns immediately without touching anything.
func Seed(ctx context.Context, st *store.Store, ev *events.Service, or *orders.Service) error {
	if _, err := st.GetOrgBySlug(ctx, DemoOrgSlug); err == nil {
		return nil // already seeded
	} else if err != store.ErrNotFound {
		return fmt.Errorf("demo: check existing seed: %w", err)
	}

	userID, err := seedUser(ctx, st)
	if err != nil {
		return fmt.Errorf("demo: seed user: %w", err)
	}

	orgID, err := seedOrg(ctx, st, userID)
	if err != nil {
		return fmt.Errorf("demo: seed org: %w", err)
	}

	now := time.Now()
	for i, se := range seedEvents() {
		eventID, err := seedOneEvent(ctx, ev, orgID, se, now)
		if err != nil {
			return fmt.Errorf("demo: seed event %q: %w", se.slug, err)
		}

		// Give the first three events a few paid orders + admissions so
		// the stats/scanner screens have something to show immediately.
		if i < 3 {
			if err := seedOrdersAndAdmissions(ctx, st, ev, or, eventID, userID, i); err != nil {
				return fmt.Errorf("demo: seed orders for %q: %w", se.slug, err)
			}
		}
	}

	return nil
}

func seedUser(ctx context.Context, st *store.Store) (string, error) {
	hash, err := auth.HashPassword(DemoPassword)
	if err != nil {
		return "", err
	}
	u := &store.User{Email: DemoEmail, PasswordHash: hash, Name: "Demo Organiser"}
	if err := st.CreateUser(ctx, u); err != nil {
		return "", err
	}
	return u.ID, nil
}

func seedOrg(ctx context.Context, st *store.Store, userID string) (string, error) {
	o := &store.Org{Name: DemoOrgName, Slug: DemoOrgSlug}
	if err := st.CreateOrg(ctx, o); err != nil {
		return "", err
	}
	// Owner: meets-or-exceeds admin and scanner too (auth.RoleMeets
	// hierarchy), so the one demo login can manage events AND scan gates.
	if err := st.AddOrgMember(ctx, &store.OrgMember{OrgID: o.ID, UserID: userID, Role: "owner"}); err != nil {
		return "", err
	}
	return o.ID, nil
}

func seedOneEvent(ctx context.Context, ev *events.Service, orgID string, se seedEvent, now time.Time) (string, error) {
	starts := now.Add(time.Duration(se.daysFromNow) * 24 * time.Hour)
	ends := starts.Add(time.Duration(se.durationHrs) * time.Hour)

	created, err := ev.Create(ctx, orgID, events.CreateEventInput{
		Slug:        se.slug,
		Title:       se.title,
		Summary:     se.summary,
		Description: se.description,
		VenueName:   se.venueName,
		Address:     se.address,
		StartsAt:    starts,
		EndsAt:      ends,
		Timezone:    "Africa/Johannesburg",
		CoverImage:  se.coverImage,
		Currency:    "ZAR",
	})
	if err != nil {
		return "", err
	}

	for _, tt := range se.ticketTypes {
		if _, err := ev.CreateTicketType(ctx, created.ID, events.TicketTypeInput{
			Name:          tt.name,
			Description:   tt.description,
			PriceCents:    tt.priceCents,
			QuantityTotal: tt.quantityTotal,
			MaxPerOrder:   10,
			Status:        "active",
		}); err != nil {
			return "", err
		}
	}

	if _, err := ev.Publish(ctx, created.ID); err != nil {
		return "", err
	}
	return created.ID, nil
}

// seedOrdersAndAdmissions places a couple of paid orders against eventID's
// first ticket type and marks one of the resulting tickets admitted, so
// the event's stats and scanner views aren't empty on first look.
func seedOrdersAndAdmissions(ctx context.Context, st *store.Store, ev *events.Service, or *orders.Service, eventID, userID string, seq int) error {
	types, err := ev.ListTicketTypes(ctx, eventID)
	if err != nil || len(types) == 0 {
		return err
	}
	ticketTypeID := types[0].ID

	buyers := []struct{ email, name string }{
		{"thandi@example.co.za", "Thandi Nkosi"},
		{"pieter@example.co.za", "Pieter van der Merwe"},
	}

	var firstTicketID string
	for i, buyer := range buyers {
		in := orders.CreateOrderInput{
			EventID:    eventID,
			BuyerEmail: buyer.email,
			BuyerName:  buyer.name,
			Items:      []orders.OrderItemInput{{TicketTypeID: ticketTypeID, Quantity: 1}},
			Provider:   "stub",
		}
		if i == 0 {
			in.UserID = userID
		}

		order, charge, err := or.Create(ctx, in)
		if err != nil {
			return err
		}

		result := payments.Result{
			Provider:    charge.Provider,
			Reference:   charge.Reference,
			EventID:     "demo-seed-" + order.ID,
			Status:      payments.StatusPaid,
			AmountCents: order.TotalCents,
			Currency:    order.Currency,
			PaidAt:      time.Now(),
		}
		_, tickets, err := or.Settle(ctx, result)
		if err != nil {
			return err
		}
		if i == 0 && len(tickets) > 0 {
			firstTicketID = tickets[0].ID
		}
	}

	// Mark the first order's ticket admitted, so this event's stats show
	// a non-zero "admitted" count and the scanner has one real duplicate
	// scenario to demonstrate if the same QR is presented twice.
	if firstTicketID != "" {
		_, err := st.DB().ExecContext(ctx, `
			INSERT INTO admissions (id, ticket_id, event_id, gate_id, scanned_by, device_id, scanned_at, result, note)
			VALUES (?, ?, ?, 'main-gate', ?, 'demo-scanner', ?, 'admitted', 'seeded for demo')`,
			store.NewID(), firstTicketID, eventID, userID, time.Now().UTC().Format(time.RFC3339),
		)
		if err != nil {
			return err
		}
	}
	return nil
}
