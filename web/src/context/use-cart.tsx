import React, { createContext, useCallback, useContext, useEffect, useMemo, useReducer, type ReactNode } from 'react';
import type { CackleEvent, TicketType } from '@/lib/api-types';

// The new API has no server-side cart (no /api/carts endpoint) — an order is
// created directly from a client-held list of {ticket_type_id, quantity} for
// a single event (see POST /api/orders in docs/API.md). So the cart here is
// purely client state, persisted to localStorage so it survives a reload,
// and never synced to the backend until checkout creates the order.

const STORAGE_KEY = 'cackle_cart_v1';

const ACTIONS = {
    ADD_ITEM: 'ADD_ITEM',
    UPDATE_QUANTITY: 'UPDATE_QUANTITY',
    REMOVE_ITEM: 'REMOVE_ITEM',
    CLEAR_EVENT: 'CLEAR_EVENT',
    CLEAR_CART: 'CLEAR_CART',
} as const;

const MAX_PER_TYPE = 10;

export interface CartItem {
    ticket_type_id: string;
    ticket_type: TicketType;
    event: CackleEvent;
    quantity: number;
}

interface CartState {
    items: CartItem[];
}

type CartAction =
    | { type: typeof ACTIONS.ADD_ITEM; payload: { ticketType: TicketType; event: CackleEvent; quantity: number } }
    | { type: typeof ACTIONS.UPDATE_QUANTITY; payload: { ticketTypeId: string; quantity: number } }
    | { type: typeof ACTIONS.REMOVE_ITEM; payload: { ticketTypeId: string } }
    | { type: typeof ACTIONS.CLEAR_EVENT; payload: { eventId: string } }
    | { type: typeof ACTIONS.CLEAR_CART };

function loadInitialState(): CartState {
    try {
        const saved = localStorage.getItem(STORAGE_KEY);
        if (!saved) return { items: [] };
        const parsed = JSON.parse(saved);
        if (!Array.isArray(parsed)) return { items: [] };
        const valid: CartItem[] = parsed.filter(
            (item) => item?.ticket_type_id && item?.event?.id && item?.ticket_type && item.quantity > 0 && item.quantity <= MAX_PER_TYPE,
        );
        return { items: valid };
    } catch {
        return { items: [] };
    }
}

function cartReducer(state: CartState, action: CartAction): CartState {
    switch (action.type) {
        case ACTIONS.ADD_ITEM: {
            const { ticketType, event, quantity } = action.payload;
            const existing = state.items.find((i) => i.ticket_type_id === ticketType.id);
            let items;
            if (existing) {
                const nextQty = Math.min(MAX_PER_TYPE, existing.quantity + quantity);
                items = state.items.map((i) => (i.ticket_type_id === ticketType.id ? { ...i, quantity: nextQty } : i));
            } else {
                items = [
                    ...state.items,
                    {
                        ticket_type_id: ticketType.id,
                        ticket_type: ticketType,
                        event,
                        quantity: Math.min(MAX_PER_TYPE, quantity),
                    },
                ];
            }
            return { items };
        }
        case ACTIONS.UPDATE_QUANTITY: {
            const { ticketTypeId, quantity } = action.payload;
            if (quantity <= 0) {
                return { items: state.items.filter((i) => i.ticket_type_id !== ticketTypeId) };
            }
            return {
                items: state.items.map((i) => (i.ticket_type_id === ticketTypeId ? { ...i, quantity: Math.min(MAX_PER_TYPE, quantity) } : i)),
            };
        }
        case ACTIONS.REMOVE_ITEM:
            return { items: state.items.filter((i) => i.ticket_type_id !== action.payload.ticketTypeId) };
        case ACTIONS.CLEAR_EVENT:
            return { items: state.items.filter((i) => i.event.id !== action.payload.eventId) };
        case ACTIONS.CLEAR_CART:
            return { items: [] };
        default:
            return state;
    }
}

export interface CartContextValue {
    items: CartItem[];
    itemsByEvent: Record<string, CartItem[]>;
    itemCount: number;
    totalsByCurrency: Record<string, number>;
    addItem: (ticketType: TicketType, event: CackleEvent, quantity?: number) => void;
    updateQuantity: (ticketTypeId: string, quantity: number) => void;
    removeItem: (ticketTypeId: string) => void;
    clearEvent: (eventId: string) => void;
    clearCart: () => void;
    eventTotal: (eventId: string) => number;
}

const CartContext = createContext<CartContextValue | null>(null);

export function CartProvider({ children }: { children: ReactNode }) {
    const [state, dispatch] = useReducer(cartReducer, undefined, loadInitialState);

    useEffect(() => {
        localStorage.setItem(STORAGE_KEY, JSON.stringify(state.items));
    }, [state.items]);

    const addItem = useCallback((ticketType: TicketType, event: CackleEvent, quantity = 1) => {
        if (!ticketType?.id || !event?.id) throw new Error('Invalid ticket type or event');
        dispatch({ type: ACTIONS.ADD_ITEM, payload: { ticketType, event, quantity: Math.max(1, quantity) } });
    }, []);

    const updateQuantity = useCallback((ticketTypeId: string, quantity: number) => {
        dispatch({ type: ACTIONS.UPDATE_QUANTITY, payload: { ticketTypeId, quantity } });
    }, []);

    const removeItem = useCallback((ticketTypeId: string) => {
        dispatch({ type: ACTIONS.REMOVE_ITEM, payload: { ticketTypeId } });
    }, []);

    const clearEvent = useCallback((eventId: string) => {
        dispatch({ type: ACTIONS.CLEAR_EVENT, payload: { eventId } });
    }, []);

    const clearCart = useCallback(() => {
        dispatch({ type: ACTIONS.CLEAR_CART });
    }, []);

    const derived = useMemo(() => {
        const itemsByEvent = state.items.reduce<Record<string, CartItem[]>>((groups, item) => {
            const id = item.event.id;
            (groups[id] ||= []).push(item);
            return groups;
        }, {});
        const itemCount = state.items.reduce((sum, i) => sum + i.quantity, 0);
        // A cart can (in principle) span multiple events, and different
        // events can be denominated in DIFFERENT currencies — Cackle has
        // no privileged currency, so there is no single meaningful "grand
        // total" to blend them into. totalsByCurrency keeps each
        // currency's minor-unit sum separate; consumers render one line
        // per currency (in the common single-event cart, that's exactly
        // one line, same as before).
        const totalsByCurrency = state.items.reduce<Record<string, number>>((acc, i) => {
            const currency = i.event?.currency || '';
            acc[currency] = (acc[currency] || 0) + i.quantity * i.ticket_type.price_minor;
            return acc;
        }, {});
        return { itemsByEvent, itemCount, totalsByCurrency };
    }, [state.items]);

    const eventTotal = useCallback(
        (eventId: string) => (derived.itemsByEvent[eventId] || []).reduce((sum, i) => sum + i.quantity * i.ticket_type.price_minor, 0),
        [derived.itemsByEvent],
    );

    const value = useMemo(
        () => ({
            items: state.items,
            itemsByEvent: derived.itemsByEvent,
            itemCount: derived.itemCount,
            totalsByCurrency: derived.totalsByCurrency,
            addItem,
            updateQuantity,
            removeItem,
            clearEvent,
            clearCart,
            eventTotal,
        }),
        [state.items, derived, addItem, updateQuantity, removeItem, clearEvent, clearCart, eventTotal],
    );

    return <CartContext.Provider value={value}>{children}</CartContext.Provider>;
}

export function useCart() {
    const ctx = useContext(CartContext);
    if (!ctx) throw new Error('useCart must be used within a CartProvider');
    return ctx;
}

export default CartProvider;
