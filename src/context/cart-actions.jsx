// cart-actions.js
import { useCallback, useContext } from 'react';
import { supabase } from '@/services/supabaseClient';
import CartContext, { ACTIONS } from './cart-context';

export function useCartActions(
  state, 
  dispatch, 
  user, 
  { syncInProgress, setSyncInProgress, setSyncError }, 
  cartOperations
) {
  const { getOrCreateCart, updateCartInDb } = cartOperations;

  const saveCart = useCallback(async () => {
    if (!user) {
      console.log('CartProvider - No user available for save');
      return null;
    }
    
    if (syncInProgress) {
      console.log('CartProvider - Sync already in progress, skipping save');
      return null;
    }

    let cartId;
    try {
      console.log('CartProvider - Starting cart save');
      setSyncInProgress(true);
      setSyncError(null);

      cartId = await getOrCreateCart(user.id);
      console.log('CartProvider - Got cart ID for save:', cartId);
      
      await updateCartInDb(cartId, state.items);
      console.log('CartProvider - Updated DB successfully');

      return cartId;
    } catch (error) {
      console.error('CartProvider - Error saving cart:', error);
      setSyncError(error);
      throw error;
    } finally {
      // Only update the cart ID if we successfully saved
      if (cartId) {
        dispatch({
          type: ACTIONS.SET_CART_ID,
          payload: { id: cartId }
        });
      }
      setSyncInProgress(false);
      console.log('CartProvider - Save complete');
    }
  }, [user, state.items, getOrCreateCart, updateCartInDb, setSyncError, setSyncInProgress, syncInProgress, dispatch]);

  const addItem = useCallback(async (ticketType, event, quantity = 1) => {
    if (!ticketType || !event) {
      console.error('CartProvider - Missing required data:', { ticketType, event });
      return;
    }
    
    if (syncInProgress) {
      console.log('CartProvider - Sync in progress, skipping add');
      return;
    }

    try {
      console.log('CartProvider - Adding item:', { ticketType, event, quantity });
      setSyncInProgress(true);
      
      // First update local state
      dispatch({ 
        type: ACTIONS.ADD_ITEM, 
        payload: {
          ticketType,
          event,
          quantity,
          unit_price: ticketType.price,
          subtotal: ticketType.price * quantity
        }
      });

      // Then save to DB if logged in
      if (user) {
        console.log('CartProvider - User present, saving cart after add');
        await saveCart();
      }
    } catch (error) {
      console.error('CartProvider - Error adding item:', error);
      setSyncError(error);
      throw error;
    } finally {
      setSyncInProgress(false);
    }
  }, [user, saveCart, setSyncInProgress, setSyncError, dispatch, syncInProgress]);

  const updateQuantity = useCallback(async (ticketTypeId, quantity) => {
    if (!ticketTypeId) {
      console.error('CartProvider - Missing ticket type ID for quantity update');
      return;
    }

    if (syncInProgress) {
      console.log('CartProvider - Sync in progress, skipping quantity update');
      return;
    }

    try {
      console.log('CartProvider - Updating quantity:', { ticketTypeId, quantity });
      setSyncInProgress(true);
      const newQuantity = Math.max(0, parseInt(quantity) || 0);
      
      // First update local state
      dispatch({ 
        type: ACTIONS.UPDATE_QUANTITY, 
        payload: { ticketTypeId, quantity: newQuantity }
      });

      // Then save to DB if logged in
      if (user) {
        console.log('CartProvider - User present, saving cart after quantity update');
        await saveCart();
      }
    } catch (error) {
      console.error('CartProvider - Error updating quantity:', error);
      setSyncError(error);
      throw error;
    } finally {
      setSyncInProgress(false);
    }
  }, [user, saveCart, setSyncInProgress, setSyncError, dispatch, syncInProgress]);

  const removeItem = useCallback(async (ticketTypeId) => {
    if (!ticketTypeId) {
      console.error('CartProvider - Missing ticket type ID for removal');
      return;
    }

    if (syncInProgress) {
      console.log('CartProvider - Sync in progress, skipping remove');
      return;
    }

    try {
      console.log('CartProvider - Removing item:', ticketTypeId);
      setSyncInProgress(true);
      
      // First update local state
      dispatch({ 
        type: ACTIONS.REMOVE_ITEM, 
        payload: { ticketTypeId }
      });

      // Then save to DB if logged in
      if (user) {
        console.log('CartProvider - User present, saving cart after removal');
        await saveCart();
      }
    } catch (error) {
      console.error('CartProvider - Error removing item:', error);
      setSyncError(error);
      throw error;
    } finally {
      setSyncInProgress(false);
    }
  }, [user, saveCart, setSyncInProgress, setSyncError, dispatch, syncInProgress]);

  const clearCart = useCallback(async () => {
    if (syncInProgress) {
      console.log('CartProvider - Sync in progress, skipping clear');
      return;
    }

    try {
      console.log('CartProvider - Clearing cart');
      setSyncInProgress(true);

      if (user && state.id) {
        console.log('CartProvider - Updating cart status to abandoned in DB');
        await supabase
          .from('carts')
          .update({ 
            status: 'abandoned',
            updated_at: new Date().toISOString()
          })
          .eq('id', state.id);
      }

      dispatch({ type: ACTIONS.CLEAR_CART });
      localStorage.removeItem('cart');
      console.log('CartProvider - Cart cleared successfully');
    } catch (error) {
      console.error('CartProvider - Error clearing cart:', error);
      setSyncError(error);
      throw error;
    } finally {
      setSyncInProgress(false);
    }
  }, [user, state.id, setSyncInProgress, setSyncError, dispatch, syncInProgress]);

  return {
    saveCart,
    addItem,
    updateQuantity,
    removeItem,
    clearCart
  };
}

export function useCart() {
  const context = useContext(CartContext);
  if (!context) {
    throw new Error('useCart must be used within a CartProvider');
  }
  return context;
}