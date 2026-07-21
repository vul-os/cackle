// Package demo seeds realistic ticketing data for `cackle --demo`, so the
// product is fully explorable with zero manual setup — the screenshotter
// and every first-time user depend on this working. Seed is idempotent:
// running it twice is a no-op the second time (detected by the presence of
// the demo org).
//
// The seeded catalogue is DELIBERATELY multi-region and multi-currency —
// most events are South African (the demo org's own home market, ZAR,
// inherited from orgs.default_currency and never set explicitly on those
// events, proving the org-default fallback works), but several are
// international with an EXPLICIT event-level currency override,
// including a zero-decimal currency (JPY — Tokyo) and a three-decimal
// currency (KWD — Kuwait City). This is the fastest way to prove the
// whole "cents was never universal" fix actually works end to end: if
// price_minor/subtotal_minor/total_minor math or the frontend's rendering
// silently assumed two decimal places anywhere, the JPY event would show
// amounts 100x too small and the KWD event would show them 1000x too
// large (or truncate the third decimal digit).
package demo

import (
	"context"
	"fmt"
	"time"

	"github.com/vul-os/cackle/internal/auth"
	"github.com/vul-os/cackle/internal/events"
	"github.com/vul-os/cackle/internal/orders"
	"github.com/vul-os/cackle/internal/orgs"
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
	category    string
	// currency is an ISO-4217 override for this event. Empty means
	// "inherit the org's default_currency" (see seedOrg) — most of the
	// South African events below deliberately leave this empty to prove
	// that fallback path works, not just the explicit-override path.
	currency    string
	timezone    string
	daysFromNow int
	durationHrs int
	ticketTypes []seedTicketType
}

type seedTicketType struct {
	name          string
	description   string
	priceMinor    int64
	quantityTotal int
}

func seedEvents() []seedEvent {
	return []seedEvent{
		{
			slug: "rocking-the-daisies", title: "Rocking the Daisies",
			summary:     "Three days of music, wine, and dust on the West Coast.",
			description: "South Africa's original boutique music festival returns to Cloof Wine Estate with four stages, a silent disco, and the best sundowner spot in the Western Cape.",
			venueName:   "Cloof Wine Estate", address: "Darling, Western Cape",
			coverImage: "/images/festival.jpg", category: "live-music", daysFromNow: 62, durationHrs: 72,
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
			coverImage: "/images/rockfest.jpg", category: "live-music", daysFromNow: 34, durationHrs: 8,
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
			coverImage: "/images/music.jpg", category: "live-music", daysFromNow: 18, durationHrs: 5,
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
			coverImage: "/images/quiz.jpg", category: "social", daysFromNow: 9, durationHrs: 3,
			ticketTypes: []seedTicketType{
				{"Team Entry (up to 6)", "One ticket covers the whole team.", 30000, 60},
			},
		},
		{
			slug: "sunrise-yoga-retreat-hermanus", title: "Sunrise Yoga Retreat",
			summary:     "A weekend of yoga, whale watching, and quiet mornings.",
			description: "Two nights in Hermanus with twice-daily yoga sessions, plant-based meals, and time to just watch the ocean.",
			venueName:   "Aloe Ridge Retreat", address: "Hermanus, Western Cape",
			coverImage: "/images/yoga.jpg", category: "wellness", daysFromNow: 45, durationHrs: 48,
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
			coverImage: "/images/racing.jpeg", category: "sports", daysFromNow: 76, durationHrs: 14,
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
			coverImage: "/images/coffee.jpg", category: "food-drink", daysFromNow: 23, durationHrs: 16,
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
			coverImage: "/images/jog.jpg", category: "sports", daysFromNow: 12, durationHrs: 4,
			ticketTypes: []seedTicketType{
				{"5km Entry", "Timed, includes race number.", 15000, 600},
				{"10km Entry", "Timed, includes race number + t-shirt.", 25000, 600},
			},
		},

		// --- International events: explicit currency override, proving
		// Cackle is genuinely country/currency-agnostic and not just
		// South Africa with a settings field. ---
		{
			slug: "shibuya-synth-nights", title: "Shibuya Synth Nights",
			summary:     "A late-night synth and city-pop lineup in the heart of Shibuya.",
			description: "Three acts, one intimate basement club, and a sound system built for analog synth. Doors open late, the last train home is not guaranteed.",
			venueName:   "Club Asia", address: "Shibuya, Tokyo, Japan",
			coverImage: "/images/music.jpg", category: "live-music", currency: "JPY", timezone: "Asia/Tokyo",
			daysFromNow: 40, durationHrs: 6,
			ticketTypes: []seedTicketType{
				// JPY has ZERO decimal places: 4500 minor IS ¥4,500, not
				// ¥45.00. If the frontend ever divides this by 100, this
				// is the ticket type that catches it.
				{"General Admission", "Standing room, all ages after 8pm.", 4500, 300},
				{"VIP Booth", "Reserved booth seating for 4, one bottle included.", 28000, 20},
			},
		},
		{
			slug: "kuwait-gulf-comedy-night", title: "Kuwait Gulf Comedy Night",
			summary:     "An English-language stand-up showcase overlooking the Gulf.",
			description: "Touring stand-up comedians from across the Gulf and further afield, seated cabaret-style with shisha and mocktails on the terrace.",
			venueName:   "Marina Crescent Terrace", address: "Salmiya, Kuwait City, Kuwait",
			coverImage: "/images/quiz.jpg", category: "social", currency: "KWD", timezone: "Asia/Kuwait",
			daysFromNow: 55, durationHrs: 4,
			ticketTypes: []seedTicketType{
				// KWD has THREE decimal places: 15500 minor IS 15.500
				// KWD, not 155.00 or 1.5500. This is the ticket type that
				// catches a hardcoded "assume 2 decimals" bug the other
				// direction from JPY.
				{"Standard Seating", "General terrace seating.", 15500, 150},
				{"Front Row", "Front-row seating + meet the lineup after the show.", 32750, 40},
			},
		},
		{
			slug: "berlin-warehouse-sessions", title: "Berlin Warehouse Sessions",
			summary:     "An all-night techno lineup in a converted Kreuzberg warehouse.",
			description: "Four rooms, twelve DJs, one warehouse. Doors at 11pm, no re-entry, cash bar only inside.",
			venueName:   "Kater Blau Nachbar", address: "Kreuzberg, Berlin, Germany",
			coverImage: "/images/festival.jpg", category: "live-music", currency: "EUR", timezone: "Europe/Berlin",
			daysFromNow: 70, durationHrs: 10,
			ticketTypes: []seedTicketType{
				{"Early Bird", "Limited early-bird allocation.", 2500, 200},
				{"General", "Standard door price.", 3500, 500},
			},
		},
		{
			slug: "brooklyn-rooftop-comedy", title: "Brooklyn Rooftop Comedy",
			summary:     "Stand-up under the skyline, rain date included.",
			description: "A rotating lineup of New York club comics on a heated rooftop with skyline views of Manhattan.",
			venueName:   "The Ludlow Rooftop", address: "Williamsburg, Brooklyn, New York, USA",
			coverImage: "/images/quiz.jpg", category: "social", currency: "USD", timezone: "America/New_York",
			daysFromNow: 27, durationHrs: 3,
			ticketTypes: []seedTicketType{
				{"General Admission", "Standing room, cash bar.", 4500, 150},
				{"Reserved Seating", "Reserved seat, one drink included.", 7500, 60},
			},
		},
	}
}

// Seed populates a demo org, a demo user, ~8 published events with ticket
// types (each with a category), a handful of paid orders with issued
// tickets, a few admissions, a second org member, a pending team invite,
// and a bank account on file — so every wave-3 surface (categories, team
// members, invites, payouts, bank account) looks alive on first run, not
// just the original ticketing flow. It is safe to call on every `--demo`
// boot: if the demo org already exists, Seed returns immediately without
// touching anything.
func Seed(ctx context.Context, st *store.Store, ev *events.Service, or *orders.Service, og *orgs.Service) error {
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

	if err := seedTeamAndPayouts(ctx, st, og, orgID, userID); err != nil {
		return fmt.Errorf("demo: seed team/payouts: %w", err)
	}

	// eventsWithOrders gets a couple of paid orders + admissions seeded so
	// its stats/scanner screens aren't empty on first look. This
	// deliberately includes the JPY and KWD events (not just the first
	// few South African ones): a full order -> settle -> issued-ticket
	// round trip through their zero-/three-decimal currencies is the
	// strongest proof this migration actually works, not just that the
	// event row itself displays a currency code.
	eventsWithOrders := map[string]bool{
		"cape-town-pub-quiz-championship": true, // the soonest event — what the
		// admin stats/attendees/scanner screens default to on first open.
		"rocking-the-daisies":      true,
		"karoo-rock-revival":       true,
		"jozi-jazz-and-soul-night": true,
		"shibuya-synth-nights":     true,
		"kuwait-gulf-comedy-night": true,
	}

	now := time.Now()
	for _, se := range seedEvents() {
		eventID, err := seedOneEvent(ctx, ev, orgID, se, now)
		if err != nil {
			return fmt.Errorf("demo: seed event %q: %w", se.slug, err)
		}

		if eventsWithOrders[se.slug] {
			if err := seedOrdersAndAdmissions(ctx, st, ev, or, eventID, userID); err != nil {
				return fmt.Errorf("demo: seed orders for %q: %w", se.slug, err)
			}
		}
	}

	return nil
}

// seedTeamAndPayouts adds a second org member, a pending team invite, and
// a bank account on file — the wave-3 organiser-console surfaces that
// have nothing to show without seed data of their own (unlike events,
// which the ticketing flow already needed). The bank account is a
// placeholder South African account number, never a real one, and
// SetBankAccount degrades gracefully with no live Paystack secret
// configured (see orgs.BankingProvider's doc) — exactly the --demo/
// self-host-without-keys path.
func seedTeamAndPayouts(ctx context.Context, st *store.Store, og *orgs.Service, orgID, ownerID string) error {
	managerHash, err := auth.HashPassword(DemoPassword)
	if err != nil {
		return err
	}
	manager := &store.User{Email: "manager@cackle.events", PasswordHash: managerHash, Name: "Demo Manager"}
	if err := st.CreateUser(ctx, manager); err != nil {
		return err
	}
	if err := st.AddOrgMember(ctx, &store.OrgMember{OrgID: orgID, UserID: manager.ID, Role: "admin"}); err != nil {
		return err
	}

	if _, _, err := og.CreateInvite(ctx, orgID, "pending-invite@cackle.events", "scanner", ownerID); err != nil {
		return err
	}

	if err := og.SetBankAccount(ctx, orgID, "051001", "62812345678", "Cackle Demo Events"); err != nil {
		return err
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
	// ZAR is this org's own choice of home-market default (it runs mostly
	// South African events) — not a privileged Cackle-wide default. Events
	// below that leave seedEvent.currency empty inherit this; the
	// international events override it explicitly.
	o := &store.Org{Name: DemoOrgName, Slug: DemoOrgSlug, DefaultCurrency: "ZAR"}
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

	timezone := se.timezone
	if timezone == "" {
		timezone = "Africa/Johannesburg"
	}

	created, err := ev.Create(ctx, orgID, events.CreateEventInput{
		Slug:        se.slug,
		Title:       se.title,
		Summary:     se.summary,
		Description: se.description,
		VenueName:   se.venueName,
		Address:     se.address,
		StartsAt:    starts,
		EndsAt:      ends,
		Timezone:    timezone,
		CoverImage:  se.coverImage,
		// Currency left empty inherits the org's own default_currency
		// (see events.Service.Create) — most South African events here
		// rely on exactly that; se.currency overrides it explicitly for
		// the international events.
		Currency: se.currency,
		Category: se.category,
	})
	if err != nil {
		return "", err
	}

	for _, tt := range se.ticketTypes {
		if _, err := ev.CreateTicketType(ctx, created.ID, events.TicketTypeInput{
			Name:          tt.name,
			Description:   tt.description,
			PriceMinor:    tt.priceMinor,
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

// demoBuyers is a pool of attendees spanning the demo's regions (SA, JP, KW)
// so seeded events read like a real crowd — a populated attendee list and
// non-trivial sales/admission stats — rather than two placeholder rows.
var demoBuyers = []struct{ email, name string }{
	{"thandi.nkosi@example.co.za", "Thandi Nkosi"},
	{"pieter.vandermerwe@example.co.za", "Pieter van der Merwe"},
	{"aisha.patel@example.co.za", "Aisha Patel"},
	{"sipho.dlamini@example.co.za", "Sipho Dlamini"},
	{"lerato.mokoena@example.co.za", "Lerato Mokoena"},
	{"johan.botha@example.co.za", "Johan Botha"},
	{"nomvula.zulu@example.co.za", "Nomvula Zulu"},
	{"kagiso.molefe@example.co.za", "Kagiso Molefe"},
	{"chantal.adams@example.co.za", "Chantal Adams"},
	{"tebogo.maluleke@example.co.za", "Tebogo Maluleke"},
	{"riaan.pretorius@example.co.za", "Riaan Pretorius"},
	{"zanele.khumalo@example.co.za", "Zanele Khumalo"},
	{"haruki.tanaka@example.jp", "Haruki Tanaka"},
	{"yuki.sato@example.jp", "Yuki Sato"},
	{"aya.watanabe@example.jp", "Aya Watanabe"},
	{"fatima.alsabah@example.kw", "Fatima Al-Sabah"},
	{"omar.alrashid@example.kw", "Omar Al-Rashid"},
	{"grace.okafor@example.co.za", "Grace Okafor"},
}

// seedOrdersAndAdmissions sells a realistic fraction of eventID's ticket
// types through the REAL order -> settle flow, then marks a majority of the
// issued tickets admitted, so the event's stats, attendees and scanner
// views demonstrate a populated event instead of an empty one. Every number
// here is real demo data produced by the actual code paths — no fabricated
// counts. It is deterministic (fixed buyer/quantity cycles, no randomness)
// so the generated screenshots are stable across runs.
func seedOrdersAndAdmissions(ctx context.Context, st *store.Store, ev *events.Service, or *orders.Service, eventID, userID string) error {
	types, err := ev.ListTicketTypes(ctx, eventID)
	if err != nil || len(types) == 0 {
		return err
	}

	// A fixed spread of order sizes so sales don't all look identical.
	qtyCycle := []int{1, 2, 1, 3, 2, 1, 2, 4}
	var issued []string
	buyerIdx, qtyIdx := 0, 0

	for _, tt := range types {
		// Aim for ~60% of capacity sold, capped so even a large-capacity
		// event (e.g. 800) seeds quickly — the loop runs a full order ->
		// settle round trip per order.
		target := tt.QuantityTotal * 6 / 10
		if target > 42 {
			target = 42
		}
		for sold := 0; sold < target; {
			qty := qtyCycle[qtyIdx%len(qtyCycle)]
			qtyIdx++
			if sold+qty > target {
				qty = target - sold
			}
			buyer := demoBuyers[buyerIdx%len(demoBuyers)]
			in := orders.CreateOrderInput{
				EventID:    eventID,
				BuyerEmail: buyer.email,
				BuyerName:  buyer.name,
				Items:      []orders.OrderItemInput{{TicketTypeID: tt.ID, Quantity: qty}},
				Provider:   "stub",
			}
			if buyerIdx == 0 {
				in.UserID = userID // the signed-in demo user owns one order
			}
			buyerIdx++

			order, charge, err := or.Create(ctx, in)
			if err != nil {
				return err
			}
			result := payments.Result{
				Provider:    charge.Provider,
				Reference:   charge.Reference,
				EventID:     "demo-seed-" + order.ID,
				Status:      payments.StatusPaid,
				AmountMinor: order.TotalMinor,
				Currency:    order.Currency,
				PaidAt:      time.Now(),
			}
			_, tickets, err := or.Settle(ctx, result)
			if err != nil {
				return err
			}
			for _, t := range tickets {
				issued = append(issued, t.ID)
			}
			sold += qty
		}
	}

	// Admit ~62% of the issued tickets ("walked through the gate"), leaving a
	// realistic no-show remainder, so stats show a meaningful admitted count
	// and admission rate. Deterministic (every ticket whose index mod 8 < 5).
	admittedAt := time.Now().UTC().Format(time.RFC3339)
	for i, ticketID := range issued {
		if i%8 >= 5 {
			continue
		}
		if _, err := st.DB().ExecContext(ctx, `
			INSERT INTO admissions (id, ticket_id, event_id, gate_id, scanned_by, device_id, scanned_at, result, note)
			VALUES (?, ?, ?, 'main-gate', ?, 'demo-scanner', ?, 'admitted', 'seeded for demo')`,
			store.NewID(), ticketID, eventID, userID, admittedAt,
		); err != nil {
			return err
		}
	}
	return nil
}
