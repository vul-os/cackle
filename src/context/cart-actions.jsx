// cart-actions.js
import { useCallback, useContext } from 'react';
import { supabase } from '@/services/supabaseClient';
import CartContext, { ACTIONS } from './cart-context';

/**
 * Custom hook for cart actions
 * @param {Object} state - Current cart state
 * @param {Function} dispatch - Cart state dispatch function
 * @param {Object} user - Current user object
 * @param {Object} syncConfig - Synchronization configuration
 * @param {boolean} syncConfig.syncInProgress - Whether sync is in progress
 * @param {Function} syncConfig.setSyncInProgress - Function to set sync status
 * @param {Function} syncConfig.setSyncError - Function to set sync error
 * @param {Function} syncConfig.syncCart - Function to trigger cart sync
 * @param {Object} cartOperations - Cart database operations
 * @returns {Object} Cart action methods
 */
export function useCartActions(
  state, 
  dispatch, 
  user, 
  { syncInProgress, setSyncInProgress, setSyncError, syncCart }, 
  cartOperations
) {
  const { getOrCreateCart, updateCartInDb } = cartOperations;

  /**
   * Force save the cart to both localStorage and database
   * @returns {Promise<void>}
   */
  const saveCart = useCallback(async () => {
    try {
      return await syncCart(true); // Force immediate sync
    } catch (error) {
      console.error('CartActions - Error in saveCart:', error);
      setSyncError(error);
      throw new Error('Failed to save cart');
    }
  }, [syncCart, setSyncError]);

  /**
   * Add item to cart
   * @param {Object} ticketType - Ticket type object
   * @param {Object} event - Event object
   * @param {number} [quantity=1] - Quantity to add
   * @returns {Promise<void>}
   */
  const addItem = useCallback(async (ticketType, event, quantity = 1) => {
    if (!ticketType?.id || !event?.id) {
      const error = new Error('Invalid ticket type or event data');
      setSyncError(error);
      throw error;
    }

    try {
      console.log('CartActions - Adding item:', { ticketType, event, quantity });
      
      // Validate quantity
      const validQuantity = Math.max(1, parseInt(quantity) || 1);
      
      // Check for existing item
      const existingItem = state.items.find(
        item => item.ticket_type_id === ticketType.id
      );
      
      if (existingItem && existingItem.quantity + validQuantity > 10) {
        const error = new Error('Cannot add more than 10 tickets per type');
        setSyncError(error);
        throw error;
      }
      
      dispatch({ 
        type: ACTIONS.ADD_ITEM, 
        payload: {
          ticketType,
          event,
          quantity: validQuantity,
          unit_price: ticketType.price,
          subtotal: ticketType.price * validQuantity
        }
      });

      // Sync will be triggered by useEffect
    } catch (error) {
      console.error('CartActions - Error adding item:', error);
      setSyncError(error);
      throw error;
    }
  }, [state.items, dispatch, setSyncError]);

  /**
   * Update item quantity
   * @param {string} ticketTypeId - Ticket type ID
   * @param {number} quantity - New quantity
   * @returns {Promise<void>}
   */
  const updateQuantity = useCallback(async (ticketTypeId, quantity) => {
    if (!ticketTypeId) {
      const error = new Error('Missing ticket type ID');
      setSyncError(error);
      throw error;
    }

    try {
      console.log('CartActions - Updating quantity:', { ticketTypeId, quantity });
      const newQuantity = Math.max(0, parseInt(quantity) || 0);
      
      // Validate quantity limit
      if (newQuantity > 10) {
        const error = new Error('Cannot exceed 10 tickets per type');
        setSyncError(error);
        throw error;
      }
      
      dispatch({ 
        type: ACTIONS.UPDATE_QUANTITY, 
        payload: { ticketTypeId, quantity: newQuantity }
      });

      // Sync will be triggered by useEffect
    } catch (error) {
      console.error('CartActions - Error updating quantity:', error);
      setSyncError(error);
      throw error;
    }
  }, [dispatch, setSyncError]);

  /**
   * Remove item from cart
   * @param {string} ticketTypeId - Ticket type ID
   * @returns {Promise<void>}
   */
  const removeItem = useCallback(async (ticketTypeId) => {
    if (!ticketTypeId) {
      const error = new Error('Missing ticket type ID');
      setSyncError(error);
      throw error;
    }

    try {
      console.log('CartActions - Removing item:', ticketTypeId);
      
      dispatch({ 
        type: ACTIONS.REMOVE_ITEM, 
        payload: { ticketTypeId }
      });

      // Sync will be triggered by useEffect
    } catch (error) {
      console.error('CartActions - Error removing item:', error);
      setSyncError(error);
      throw error;
    }
  }, [dispatch, setSyncError]);

  /**
   * Clear the entire cart
   * @returns {Promise<void>}
   */
  const clearCart = useCallback(async () => {
    try {
      console.log('CartActions - Clearing cart');

      if (user && state.id) {
        console.log('CartActions - Updating cart status to abandoned in DB');
        const { error: updateError } = await supabase
          .from('carts')
          .update({ 
            status: 'abandoned',
            updated_at: new Date().toISOString()
          })
          .eq('id', state.id);

        if (updateError) throw updateError;
      }

      dispatch({ type: ACTIONS.CLEAR_CART });
      localStorage.removeItem('cart');
      console.log('CartActions - Cart cleared successfully');
    } catch (error) {
      console.error('CartActions - Error clearing cart:', error);
      setSyncError(error);
      throw error;
    }
  }, [user, state.id, dispatch, setSyncError]);

  /**
   * Get cart items for a specific event
   * @param {string} eventId - Event ID
   * @returns {Array} Array of cart items for the event
   */
  const getEventItems = useCallback((eventId) => {
    return state.items.filter(item => item.event_id === eventId);
  }, [state.items]);

  /**
   * Calculate total quantity for a specific event
   * @param {string} eventId - Event ID
   * @returns {number} Total quantity
   */
  const getEventQuantity = useCallback((eventId) => {
    return state.items
      .filter(item => item.event_id === eventId)
      .reduce((sum, item) => sum + (item.quantity || 0), 0);
  }, [state.items]);

  /**
   * Calculate total price for a specific event
   * @param {string} eventId - Event ID
   * @returns {number} Total price
   */
  const getEventTotal = useCallback((eventId) => {
    return state.items
      .filter(item => item.event_id === eventId)
      .reduce((sum, item) => sum + (item.subtotal || 0), 0);
  }, [state.items]);

  /**
   * Check if a specific ticket type is in the cart
   * @param {string} ticketTypeId - Ticket type ID
   * @returns {boolean} Whether the ticket type is in cart
   */
  const hasTicketType = useCallback((ticketTypeId) => {
    return state.items.some(item => item.ticket_type_id === ticketTypeId);
  }, [state.items]);

  /**
   * Get quantity of a specific ticket type in cart
   * @param {string} ticketTypeId - Ticket type ID
   * @returns {number} Quantity in cart
   */
  const getTicketTypeQuantity = useCallback((ticketTypeId) => {
    const item = state.items.find(item => item.ticket_type_id === ticketTypeId);
    return item ? item.quantity : 0;
  }, [state.items]);

  return {
    // Core cart operations
    saveCart,
    addItem,
    updateQuantity,
    removeItem,
    clearCart,
    
    // Helper methods
    getEventItems,
    getEventQuantity,
    getEventTotal,
    hasTicketType,
    getTicketTypeQuantity,
    
    // Sync status
    syncInProgress
  };
}

/**
 * Hook to access cart context and actions
 * @returns {Object} Cart context and actions
 * @throws {Error} If used outside CartProvider
 */
export function useCart() {
  const context = useContext(CartContext);
  if (!context) {
    throw new Error('useCart must be used within a CartProvider');
  }
  return context;
}