package events

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/vul-os/cackle/internal/store"
)

// TicketType is one class of ticket sold for an event.
type TicketType struct {
	ID            string     `json:"id"`
	EventID       string     `json:"event_id"`
	Name          string     `json:"name"`
	Description   string     `json:"description"`
	PriceMinor    int64      `json:"price_minor"`
	QuantityTotal int        `json:"quantity_total"`
	QuantitySold  int        `json:"quantity_sold"`
	SalesStart    *time.Time `json:"sales_start,omitempty"`
	SalesEnd      *time.Time `json:"sales_end,omitempty"`
	MaxPerOrder   int        `json:"max_per_order"`
	Status        string     `json:"status"`
	SortOrder     int        `json:"sort_order"`
}

// TicketTypeInput is the input to both CreateTicketType and
// UpdateTicketType — Update is a full replace of every editable field
// (everything here except QuantitySold, which callers can never set
// directly).
type TicketTypeInput struct {
	Name          string     `json:"name"`
	Description   string     `json:"description"`
	PriceMinor    int64      `json:"price_minor"`
	QuantityTotal int        `json:"quantity_total"`
	SalesStart    *time.Time `json:"sales_start,omitempty"`
	SalesEnd      *time.Time `json:"sales_end,omitempty"`
	MaxPerOrder   int        `json:"max_per_order"`
	Status        string     `json:"status,omitempty"`
	SortOrder     int        `json:"sort_order"`
}

func (in TicketTypeInput) validate() error {
	if strings.TrimSpace(in.Name) == "" {
		return fmt.Errorf("%w: name is required", ErrInvalidInput)
	}
	if in.PriceMinor < 0 {
		return fmt.Errorf("%w: price_minor cannot be negative", ErrInvalidInput)
	}
	if in.QuantityTotal < 0 {
		return fmt.Errorf("%w: quantity_total cannot be negative", ErrInvalidInput)
	}
	if in.MaxPerOrder < 0 {
		return fmt.Errorf("%w: max_per_order cannot be negative", ErrInvalidInput)
	}
	if in.SalesStart != nil && in.SalesEnd != nil && !in.SalesEnd.After(*in.SalesStart) {
		return fmt.Errorf("%w: sales_end must be after sales_start", ErrInvalidInput)
	}
	return nil
}

// CreateTicketType creates a new ticket type for an event, starting with
// zero sold.
func (s *Service) CreateTicketType(ctx context.Context, eventID string, in TicketTypeInput) (*TicketType, error) {
	if err := in.validate(); err != nil {
		return nil, err
	}
	if _, err := s.store.GetEventByID(ctx, eventID); err != nil {
		return nil, err
	}

	tt := &store.TicketType{
		ID:            store.NewID(),
		EventID:       eventID,
		Name:          in.Name,
		Description:   in.Description,
		PriceMinor:    in.PriceMinor,
		QuantityTotal: in.QuantityTotal,
		SalesStart:    in.SalesStart,
		SalesEnd:      in.SalesEnd,
		MaxPerOrder:   in.MaxPerOrder,
		Status:        defaultStr(in.Status, "active"),
		SortOrder:     in.SortOrder,
	}
	if err := s.store.CreateTicketType(ctx, tt); err != nil {
		return nil, fmt.Errorf("events: create ticket type: %w", err)
	}
	out := toTicketType(tt)
	return &out, nil
}

// UpdateTicketType replaces every editable field of an existing ticket
// type. QuantityTotal may not be set below the number of units already
// sold/reserved (ErrQuantityBelowSold).
func (s *Service) UpdateTicketType(ctx context.Context, id string, in TicketTypeInput) (*TicketType, error) {
	if err := in.validate(); err != nil {
		return nil, err
	}

	existing, err := s.store.GetTicketTypeByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if in.QuantityTotal < existing.QuantitySold {
		return nil, fmt.Errorf("%w: %d < %d already sold", ErrQuantityBelowSold, in.QuantityTotal, existing.QuantitySold)
	}

	tt := &store.TicketType{
		ID:            existing.ID,
		EventID:       existing.EventID,
		Name:          in.Name,
		Description:   in.Description,
		PriceMinor:    in.PriceMinor,
		QuantityTotal: in.QuantityTotal,
		QuantitySold:  existing.QuantitySold,
		SalesStart:    in.SalesStart,
		SalesEnd:      in.SalesEnd,
		MaxPerOrder:   in.MaxPerOrder,
		Status:        defaultStr(in.Status, "active"),
		SortOrder:     in.SortOrder,
	}
	if err := s.store.UpdateTicketType(ctx, tt); err != nil {
		return nil, fmt.Errorf("events: update ticket type: %w", err)
	}
	out := toTicketType(tt)
	return &out, nil
}

// DeleteTicketType removes a ticket type. Rejected with
// ErrTicketTypeHasSales if any inventory has already been reserved/sold —
// deleting it would either orphan existing orders or (thanks to the
// foreign key constraint on order_items.ticket_type_id) fail at the
// database level anyway; this gives a clean domain error instead of a raw
// SQL constraint violation.
func (s *Service) DeleteTicketType(ctx context.Context, id string) error {
	tt, err := s.store.GetTicketTypeByID(ctx, id)
	if err != nil {
		return err
	}
	if tt.QuantitySold > 0 {
		return fmt.Errorf("%w: %d sold/reserved", ErrTicketTypeHasSales, tt.QuantitySold)
	}
	if err := s.store.DeleteTicketType(ctx, id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return err
		}
		return fmt.Errorf("events: delete ticket type: %w", err)
	}
	return nil
}

// ListTicketTypes returns every ticket type for an event, in display
// order.
func (s *Service) ListTicketTypes(ctx context.Context, eventID string) ([]TicketType, error) {
	rows, err := s.store.ListTicketTypesForEvent(ctx, eventID)
	if err != nil {
		return nil, err
	}
	out := make([]TicketType, len(rows))
	for i := range rows {
		out[i] = toTicketType(&rows[i])
	}
	return out, nil
}

func toTicketType(tt *store.TicketType) TicketType {
	return TicketType{
		ID:            tt.ID,
		EventID:       tt.EventID,
		Name:          tt.Name,
		Description:   tt.Description,
		PriceMinor:    tt.PriceMinor,
		QuantityTotal: tt.QuantityTotal,
		QuantitySold:  tt.QuantitySold,
		SalesStart:    tt.SalesStart,
		SalesEnd:      tt.SalesEnd,
		MaxPerOrder:   tt.MaxPerOrder,
		Status:        tt.Status,
		SortOrder:     tt.SortOrder,
	}
}
