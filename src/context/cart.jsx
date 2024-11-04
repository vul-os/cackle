import React, { createContext, useContext, useReducer, useEffect, useState } from 'react';
import { supabase } from '@/services/supabaseClient';
import { AuthContext } from './use-auth';

const CartContext = createContext();

const ACTIONS = {
  ADD_ITEM: 'ADD_ITEM',
  UPDATE_QUANTITY: 'UPDATE_QUANTITY',
  REMOVE_ITEM: 'REMOVE_ITEM',
  CLEAR_CART: 'CLEAR_CART',
  SET_CART: 'SET_CART',
};

const cartReducer = (state, action) => {
  switch (action.type) {
    case ACTIONS.SET_CART: {
      return {
        ...state,
        id: action.payload.id,
        items: action.payload.items,
      };
    }
    case ACTIONS.ADD_ITEM: {
      const { ticketType, quantity } = action.payload;
      console.log('Adding item:', ticketType, 'quantity:', quantity);
      
      const existingItem = state.items.find(
        item => item.ticket_type_id === ticketType.id
      );

      if (existingItem) {
        return {
          ...state,
          items: state.items.map(item =>
            item.ticket_type_id === ticketType.id
              ? {
                  ...item,
                  quantity: item.quantity + quantity,
                  subtotal: (item.quantity + quantity) * item.unit_price,
                }
              : item
          ),
        };
      }

      const newItem = {
        ticket_type_id: ticketType.id,
        ticket_type: ticketType,
        quantity,
        unit_price: ticketType.price,
        discount_amount: 0,
        subtotal: quantity * ticketType.price,
      };

      return {
        ...state,
        items: [...state.items, newItem],
      };
    }

    case ACTIONS.UPDATE_QUANTITY: {
      const { itemId, quantity } = action.payload;
      if (quantity <= 0) {
        return {
          ...state,
          items: state.items.filter(item => item.id !== itemId),
        };
      }

      return {
        ...state,
        items: state.items.map(item =>
          item.id === itemId
            ? {
                ...item,
                quantity,
                subtotal: quantity * item.unit_price,
              }
            : item
        ),
      };
    }

    case ACTIONS.REMOVE_ITEM: {
      return {
        ...state,
        items: state.items.filter(item => item.id !== action.payload.itemId),
      };
    }

    case ACTIONS.CLEAR_CART: {
      return {
        ...state,
        id: null,
        items: [],
      };
    }

    default:
      return state;
  }
};

export function CartProvider({ children }) {
  const { user, organization } = useContext(AuthContext);
  const [previousUser, setPreviousUser] = useState(null);

  const initialState = {
    id: null,
    items: JSON.parse(localStorage.getItem('cart') || '[]'),
  };

  const [state, dispatch] = useReducer(cartReducer, initialState);

  // Calculate derived values
  const itemCount = state.items.reduce((sum, item) => sum + item.quantity, 0);
  const total = state.items.reduce(
    (sum, item) => sum + item.subtotal - item.discount_amount,
    0
  );

  // Watch for user login and migrate localStorage cart to DB
  useEffect(() => {
    const migrateCartToDb = async () => {
      // Only run when user first logs in
      if (!previousUser && user && organization) {
        try {
          console.log('User logged in, migrating localStorage cart to DB');
          
          // Create new cart
          const { data: newCart, error: createError } = await supabase
            .from('carts')
            .insert({
              profile_id: user.id,
              organization_id: organization.id,
              status: 'active'
            })
            .select('id')
            .single();

          if (createError) {
            console.error('Error creating cart:', createError);
            return;
          }

          // Insert local items into DB if they exist
          if (state.items.length > 0) {
            const cartItems = state.items.map(item => ({
              cart_id: newCart.id,
              organization_id: organization.id,
              ticket_type_id: item.ticket_type_id,
              quantity: item.quantity,
              unit_price: item.unit_price,
              discount_amount: item.discount_amount || 0,
              subtotal: item.subtotal
            }));

            const { error: insertError } = await supabase
              .from('cart_items')
              .insert(cartItems);

            if (insertError) {
              console.error('Error inserting cart items:', insertError);
              return;
            }
          }

          // Update state with new cart ID
          dispatch({
            type: ACTIONS.SET_CART,
            payload: {
              id: newCart.id,
              items: state.items
            }
          });

        } catch (error) {
          console.error('Error migrating cart to DB:', error);
        }
      }

      setPreviousUser(user);
    };

    migrateCartToDb();
  }, [user, organization]);

  // Update localStorage whenever cart items change
  useEffect(() => {
    localStorage.setItem('cart', JSON.stringify(state.items));
  }, [state.items]);

  // Update DB if user is logged in and cart items change
  useEffect(() => {
    const updateDbCart = async () => {
      if (!user || !state.id || !organization) return;

      try {
        console.log('Updating DB cart:', state.items);

        // Delete existing cart items
        await supabase
          .from('cart_items')
          .delete()
          .eq('cart_id', state.id);

        if (state.items.length > 0) {
          const cartItems = state.items.map(item => ({
            cart_id: state.id,
            organization_id: organization.id,
            ticket_type_id: item.ticket_type_id,
            quantity: item.quantity,
            unit_price: item.unit_price,
            discount_amount: item.discount_amount || 0,
            subtotal: item.subtotal
          }));

          const { error: insertError } = await supabase
            .from('cart_items')
            .insert(cartItems);

          if (insertError) {
            console.error('Error updating cart items:', insertError);
          }
        }

        // Update cart timestamp
        await supabase
          .from('carts')
          .update({ updated_at: new Date().toISOString() })
          .eq('id', state.id);

      } catch (error) {
        console.error('Error updating DB cart:', error);
      }
    };

    if (state.id) {
      updateDbCart();
    }
  }, [state.items, user, organization]);

  const addItem = (ticketType, quantity) => {
    console.log('Adding item to cart:', ticketType, quantity);
    dispatch({ type: ACTIONS.ADD_ITEM, payload: { ticketType, quantity } });
  };

  const updateQuantity = (itemId, quantity) => {
    console.log('Updating quantity:', itemId, quantity);
    dispatch({ type: ACTIONS.UPDATE_QUANTITY, payload: { itemId, quantity } });
  };

  const removeItem = (itemId) => {
    console.log('Removing item:', itemId);
    dispatch({ type: ACTIONS.REMOVE_ITEM, payload: { itemId } });
  };

  const clearCart = async () => {
    if (user && state.id) {
      try {
        await supabase
          .from('carts')
          .update({ status: 'abandoned' })
          .eq('id', state.id);
      } catch (error) {
        console.error('Error abandoning cart:', error);
      }
    }
    dispatch({ type: ACTIONS.CLEAR_CART });
  };

  const value = {
    id: state.id,
    items: state.items,
    itemCount,
    total,
    addItem,
    updateQuantity,
    removeItem,
    clearCart,
  };

  return <CartContext.Provider value={value}>{children}</CartContext.Provider>;
}

export function useCart() {
  const context = useContext(CartContext);
  if (!context) {
    throw new Error('useCart must be used within a CartProvider');
  }
  return context;
}

export default CartContext;