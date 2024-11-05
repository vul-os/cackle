// cart-context.js
import { createContext } from 'react';

export const ACTIONS = {
  SET_CART: 'SET_CART',
  SET_CART_ID: 'SET_CART_ID',
  ADD_ITEM: 'ADD_ITEM',
  UPDATE_QUANTITY: 'UPDATE_QUANTITY',
  REMOVE_ITEM: 'REMOVE_ITEM',
  CLEAR_CART: 'CLEAR_CART',
};

export function cartReducer(state, action) {
  console.log('Cart Reducer - Action:', action.type, action.payload);
  
  switch (action.type) {
    case ACTIONS.SET_CART:
      return {
        id: action.payload.id,
        items: action.payload.items
      };
      
    case ACTIONS.SET_CART_ID:
      return {
        ...state,
        id: action.payload.id
      };

    case ACTIONS.ADD_ITEM: {
      const { ticketType, event, quantity, unit_price, subtotal } = action.payload;
      const existingItem = state.items.find(
        item => item.ticket_type_id === ticketType.id
      );

      let newItems;
      if (existingItem) {
        newItems = state.items.map(item =>
          item.ticket_type_id === ticketType.id
            ? {
                ...item,
                quantity: item.quantity + quantity,
                subtotal: (item.quantity + quantity) * item.unit_price
              }
            : item
        );
      } else {
        newItems = [
          ...state.items,
          {
            ticket_type_id: ticketType.id,
            ticket_type: ticketType,
            event_id: event.id,
            event: event,
            quantity,
            unit_price,
            subtotal
          }
        ];
      }

      return {
        ...state,
        items: newItems
      };
    }

    case ACTIONS.UPDATE_QUANTITY: {
      const { ticketTypeId, quantity } = action.payload;
      
      if (quantity === 0) {
        return {
          ...state,
          items: state.items.filter(item => item.ticket_type_id !== ticketTypeId)
        };
      }

      const newItems = state.items.map(item =>
        item.ticket_type_id === ticketTypeId
          ? {
              ...item,
              quantity,
              subtotal: quantity * item.unit_price
            }
          : item
      );

      return {
        ...state,
        items: newItems
      };
    }

    case ACTIONS.REMOVE_ITEM: {
      return {
        ...state,
        items: state.items.filter(item => item.ticket_type_id !== action.payload.ticketTypeId)
      };
    }

    case ACTIONS.CLEAR_CART:
      return {
        id: null,
        items: []
      };

    default:
      return state;
  }
}

const CartContext = createContext();
export default CartContext;