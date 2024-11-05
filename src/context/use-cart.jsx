// cart-provider.jsx
import React, { useReducer, useEffect, useState, useContext } from 'react';
import { AuthContext } from './use-auth';
import CartContext, { cartReducer, ACTIONS } from './cart-context';
import { useCartOperations } from './cart-operations';
import { useCartActions } from './cart-actions';
import { supabase } from '@/services/supabaseClient';

// Utility function to load and validate cart from localStorage
const loadCartFromLocalStorage = () => {
  try {
    const savedCart = localStorage.getItem('cart');
    console.log('CartProvider - Initial load from localStorage:', savedCart);
    
    if (savedCart) {
      const parsedCart = JSON.parse(savedCart);
      if (Array.isArray(parsedCart)) {
        const validItems = parsedCart.filter(item => {
          const isValid = item && 
            item.ticket_type_id && 
            item.quantity && 
            item.quantity > 0 &&
            item.ticket_type &&
            item.event;
          
          if (!isValid) {
            console.warn('CartProvider - Invalid item filtered out:', item);
          }
          return isValid;
        });
        
        console.log('CartProvider - Valid items loaded:', validItems);
        return {
          id: null,
          items: validItems
        };
      }
    }
  } catch (error) {
    console.error('CartProvider - Error loading cart from localStorage:', error);
    localStorage.removeItem('cart');
  }
  console.log('CartProvider - Initializing empty cart');
  return { id: null, items: [] };
};

export function CartProvider({ children }) {
  const { user } = useContext(AuthContext);
  const [previousUser, setPreviousUser] = useState(null);
  const [syncInProgress, setSyncInProgress] = useState(false);
  const [isLoading, setIsLoading] = useState(false); // Start with false
  const [syncError, setSyncError] = useState(null);
  
  // Initialize state from localStorage
  const [state, dispatch] = useReducer(cartReducer, undefined, loadCartFromLocalStorage);

  // Initial load effect
  useEffect(() => {
    const initializeCart = async () => {
      if (user) {
        setIsLoading(true);
        try {
          await loadCartFromDb();
        } catch (error) {
          console.error('Error loading cart:', error);
          setSyncError(error);
        } finally {
          setIsLoading(false);
        }
      }
    };

    initializeCart();
  }, []); // Run once on mount

  // Add safety timeout for syncInProgress
  useEffect(() => {
    if (syncInProgress) {
      console.log('CartProvider - Sync in progress...');
      const timeout = setTimeout(() => {
        console.warn('CartProvider - Sync timeout reached, resetting syncInProgress');
        setSyncInProgress(false);
      }, 10000); // 10 second safety timeout
      return () => clearTimeout(timeout);
    }
  }, [syncInProgress]);

  // Calculate derived values
  const itemCount = state.items.reduce((sum, item) => sum + (item.quantity || 0), 0);
  const total = state.items.reduce((sum, item) => sum + (item.subtotal || 0), 0);
  const itemsByEvent = state.items.reduce((groups, item) => {
    const eventId = item.event_id || item.event?.id;
    if (!eventId) {
      console.warn('CartProvider - Item missing event ID:', item);
      return groups;
    }
    if (!groups[eventId]) {
      groups[eventId] = [];
    }
    groups[eventId].push(item);
    return groups;
  }, {});

  // Persist cart to localStorage whenever items change
  useEffect(() => {
    if (syncInProgress) {
      console.log('CartProvider - Skipping localStorage save during sync');
      return;
    }
    
    const timeoutId = setTimeout(() => {
      try {
        console.log('CartProvider - Saving to localStorage:', state.items);
        
        // Filter out invalid items before saving
        const itemsToSave = state.items.filter(item => {
          const isValid = item && 
            item.ticket_type_id && 
            item.quantity && 
            item.quantity > 0 &&
            item.ticket_type &&
            item.event;
          
          if (!isValid) {
            console.warn('CartProvider - Invalid item filtered out before save:', item);
          }
          return isValid;
        });
        
        localStorage.setItem('cart', JSON.stringify(itemsToSave));
        console.log('CartProvider - Successfully saved to localStorage');
      } catch (error) {
        console.error('CartProvider - Error saving cart to localStorage:', error);
      }
    }, 0);

    return () => clearTimeout(timeoutId);
  }, [state.items, syncInProgress]);

  const cartOperations = useCartOperations();
  const { transformCartItems, mergeCartItems } = cartOperations;

  const loadCartFromDb = async () => {
    if (!user) {
      console.log('CartProvider - No user available for load, using localStorage cart');
      return;
    }
    
    if (syncInProgress) {
      console.log('CartProvider - Sync in progress, skipping load');
      return;
    }
    
    try {
      console.log('CartProvider - Starting cart load');
      setSyncInProgress(true);
      setSyncError(null);
      
      const localItems = [...state.items];
      console.log('CartProvider - Local items before load:', localItems);
      
      const { data: carts, error: cartError } = await supabase
        .from('carts')
        .select('id')
        .eq('profile_id', user.id)
        .eq('status', 'active')
        .order('created_at', { ascending: false })
        .limit(1);
        
      if (cartError) {
        console.error('CartProvider - Error fetching cart:', cartError);
        throw cartError;
      }
      
      let dbItems = [];
      const cart = carts?.[0];
      console.log('CartProvider - Found cart:', cart);

      if (cart) {
        const { data: cartItems, error: itemsError } = await supabase
          .from('cart_items')
          .select(`
            id,
            ticket_type_id,
            quantity,
            unit_price,
            discount_amount,
            subtotal,
            ticket_type:ticket_types!inner(
              id,
              name,
              price,
              event:events!inner(
                id,
                title,
                start_time,
                end_time,
                venue_name,
                venue_address
              )
            )
          `)
          .eq('cart_id', cart.id);
          
        if (itemsError) {
          console.error('CartProvider - Error fetching cart items:', itemsError);
          throw itemsError;
        }
        
        console.log('CartProvider - Raw cart items from DB:', cartItems);
        dbItems = transformCartItems(cartItems);
        console.log('CartProvider - Transformed DB items:', dbItems);
      }

      const mergedItems = mergeCartItems(localItems, dbItems);
      console.log('CartProvider - Merged items:', mergedItems);

      dispatch({
        type: ACTIONS.SET_CART,
        payload: { id: cart?.id || null, items: mergedItems }
      });

      if (cart && mergedItems.length !== dbItems.length) {
        console.log('CartProvider - Updating DB with merged items');
        await cartOperations.updateCartInDb(cart.id, mergedItems);
      } else if (!cart && mergedItems.length > 0) {
        console.log('CartProvider - Creating new cart for merged items');
        const newCartId = await cartOperations.getOrCreateCart(user.id);
        await cartOperations.updateCartInDb(newCartId, mergedItems);
        dispatch({
          type: ACTIONS.SET_CART,
          payload: { id: newCartId, items: mergedItems }
        });
      }

    } catch (error) {
      console.error('CartProvider - Error loading cart:', error);
      setSyncError(error);
      // Fall back to local storage cart on error
      dispatch({
        type: ACTIONS.SET_CART,
        payload: loadCartFromLocalStorage()
      });
    } finally {
      setSyncInProgress(false);
      setIsLoading(false);
      console.log('CartProvider - Load complete');
    }
  };

  // Handle user changes
  useEffect(() => {
    const handleUserChange = async () => {
      if (syncInProgress) {
        console.log('CartProvider - User change ignored due to sync in progress');
        return;
      }
      
      try {
        console.log('CartProvider - Handling user change', { 
          currentUser: user?.id, 
          previousUser: previousUser?.id 
        });
        
        if (user && (!previousUser || previousUser.id !== user.id)) {
          console.log('CartProvider - New user detected, loading cart from DB');
          setIsLoading(true);
          await loadCartFromDb();
        }
        else if (!user && previousUser) {
          console.log('CartProvider - User logged out, loading localStorage cart');
          dispatch({ 
            type: ACTIONS.SET_CART, 
            payload: loadCartFromLocalStorage()
          });
        }
      } catch (error) {
        console.error('CartProvider - Error handling user change:', error);
        setSyncError(error);
      } finally {
        setPreviousUser(user);
        setIsLoading(false);
      }
    };

    handleUserChange();
  }, [user, previousUser, syncInProgress]);

  const cartActions = useCartActions(
    state,
    dispatch,
    user,
    { 
      syncInProgress,
      setSyncInProgress,
      setSyncError
    },
    cartOperations
  );

  const value = {
    id: state.id,
    items: state.items,
    itemsByEvent,
    itemCount,
    total,
    isLoading,
    syncInProgress,
    syncError,
    ...cartActions
  };

  return <CartContext.Provider value={value}>{children}</CartContext.Provider>;
}

export { useCart } from './cart-actions';
export default CartProvider;