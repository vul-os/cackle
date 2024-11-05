// cart-provider.jsx
import React, { useReducer, useEffect, useState, useContext, useMemo } from 'react';
import { AuthContext } from './use-auth';
import CartContext, { cartReducer, ACTIONS } from './cart-context';
import { useCartOperations } from './cart-operations';
import { useCartActions } from './cart-actions';
import { supabase } from '@/services/supabaseClient';

const loadCartFromLocalStorage = () => {
  try {
    const savedCart = localStorage.getItem('cart');
    if (savedCart) {
      const parsedCart = JSON.parse(savedCart);
      if (Array.isArray(parsedCart)) {
        const validItems = parsedCart.filter(item => {
          const isValid = item && 
            item.ticket_type_id && 
            item.quantity && 
            item.quantity > 0 &&
            item.quantity <= 10 && 
            item.ticket_type &&
            item.event &&
            item.unit_price &&
            item.subtotal;
          return isValid;
        });
        return { id: null, items: validItems };
      }
    }
  } catch (error) {
    console.error('Error loading cart:', error);
    localStorage.removeItem('cart');
  }
  return { id: null, items: [] };
};

export function CartProvider({ children }) {
  const { user } = useContext(AuthContext);
  const [isLoading, setIsLoading] = useState(true);
  const [syncInProgress, setSyncInProgress] = useState(false);
  const [syncError, setSyncError] = useState(null);
  
  const [state, dispatch] = useReducer(cartReducer, undefined, loadCartFromLocalStorage);
  const cartOperations = useCartOperations();

  // Simple function to update DB with local cart
  const updateDBWithLocal = async () => {
    if (!user) return;
    
    try {
      setSyncInProgress(true);
      
      // Abandon existing carts
      await supabase
        .from('carts')
        .update({ status: 'abandoned' })
        .eq('profile_id', user.id)
        .eq('status', 'active');

      // If no items, we're done
      if (!state.items.length) {
        dispatch({ type: ACTIONS.SET_CART_ID, payload: { id: null } });
        return;
      }

      // Create new cart
      const { data: cart } = await supabase
        .from('carts')
        .insert({ profile_id: user.id, status: 'active' })
        .select('id')
        .single();

      // Add items
      await supabase
        .from('cart_items')
        .insert(state.items.map(item => ({
          cart_id: cart.id,
          ticket_type_id: item.ticket_type_id,
          quantity: item.quantity,
          unit_price: item.unit_price,
          subtotal: item.subtotal
        })));

      dispatch({ type: ACTIONS.SET_CART_ID, payload: { id: cart.id } });
      
    } catch (error) {
      console.error('Error updating DB:', error);
      setSyncError(error);
    } finally {
      setSyncInProgress(false);
    }
  };

  // Update localStorage whenever items change
  useEffect(() => {
    localStorage.setItem('cart', JSON.stringify(state.items));
  }, [state.items]);

  // Update DB when logged in and items change
  useEffect(() => {
    if (user && !syncInProgress) {
      updateDBWithLocal();
    }
  }, [user, state.items]);

  // Handle login/logout
  useEffect(() => {
    if (user) {
      updateDBWithLocal();
    } else {
      dispatch({ type: ACTIONS.SET_CART_ID, payload: { id: null } });
    }
    setIsLoading(false);
  }, [user]);

  const cartState = useMemo(() => ({
    itemCount: state.items.reduce((sum, item) => sum + (item.quantity || 0), 0),
    total: state.items.reduce((sum, item) => sum + (item.subtotal || 0), 0),
    itemsByEvent: state.items.reduce((groups, item) => {
      const eventId = item.event_id || item.event?.id;
      if (!eventId) return groups;
      if (!groups[eventId]) groups[eventId] = [];
      groups[eventId].push(item);
      return groups;
    }, {})
  }), [state.items]);

  const cartActions = useCartActions(
    state,
    dispatch,
    user,
    { syncInProgress, setSyncInProgress, setSyncError },
    cartOperations
  );

  const value = {
    id: state.id,
    items: state.items,
    itemsByEvent: cartState.itemsByEvent,
    itemCount: cartState.itemCount,
    total: cartState.total,
    isLoading,
    syncInProgress,
    syncError,
    ...cartActions
  };

  return <CartContext.Provider value={value}>{children}</CartContext.Provider>;
}

export { useCart } from './cart-actions';
export default CartProvider;