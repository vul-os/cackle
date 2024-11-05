import { createContext, useContext, useReducer, useCallback, useEffect, useMemo } from 'react';
import { supabase } from '@/services/supabaseClient';
import { useCart } from './use-cart';
import { AuthContext } from './use-auth';

const OrderContext = createContext(null);

export const ACTIONS = {
  SET_ORDER: 'SET_ORDER',
  UPDATE_ORDER: 'UPDATE_ORDER',
  CLEAR_ORDER: 'CLEAR_ORDER',
};

function orderReducer(state, action) {
  console.log('Order Reducer - Action:', action.type, action.payload);

  switch (action.type) {
    case ACTIONS.SET_ORDER:
      return action.payload ? { ...action.payload } : null;

    case ACTIONS.UPDATE_ORDER:
      return state ? { ...state, ...action.payload } : null;

    case ACTIONS.CLEAR_ORDER:
      return null;

    default:
      return state;
  }
}

function useOrderOperations(dispatch, cartItems, clearCart, orderState) {
  const fetchActiveOrder = useCallback(async (profileId) => {
    const { data: order, error } = await supabase
      .from('orders')
      .select(`
        *,
        order_items (
          *,
          ticket_type:ticket_types (
            *,
            event:events (*)
          )
        )
      `)
      .eq('profile_id', profileId)
      .in('status', ['pending', 'processing'])
      .order('created_at', { ascending: false })
      .limit(1)
      .maybeSingle();

    if (error) throw error;
    return order;
  }, []);

  const getOrder = useCallback(async (orderId) => {
    const { data: order, error } = await supabase
      .from('orders')
      .select(`
        *,
        order_items (
          *,
          ticket_type:ticket_types (
            *,
            event:events (*)
          )
        )
      `)
      .eq('id', orderId)
      .single();

    if (error) throw error;
    return order;
  }, []);

  const updateOrder = useCallback(async (orderId, updates) => {
    if (!orderId) {
      throw new Error('Order ID is required');
    }

    const { data: updatedOrder, error } = await supabase
      .from('orders')
      .update({
        ...updates,
        updated_at: new Date().toISOString()
      })
      .eq('id', orderId)
      .select(`
        *,
        order_items (
          *,
          ticket_type:ticket_types (
            *,
            event:events (*)
          )
        )
      `)
      .single();

    if (error) throw error;
    return updatedOrder;
  }, []);

  const createOrder = useCallback(async (profileId) => {
    if (!cartItems.length) {
      throw new Error('Cart is empty');
    }

    try {
      const totalAmount = cartItems.reduce((sum, item) => sum + item.subtotal, 0);
      
      const { data: order, error: orderError } = await supabase
        .from('orders')
        .insert({
          profile_id: profileId,
          status: 'pending',
          total_amount: totalAmount,
          currency: 'USD',
          created_at: new Date().toISOString(),
          updated_at: new Date().toISOString()
        })
        .select()
        .single();

      if (orderError) throw orderError;

      const orderItems = cartItems.map(item => ({
        order_id: order.id,
        ticket_type_id: item.ticket_type_id,
        quantity: item.quantity,
        unit_price: item.unit_price,
        subtotal: item.subtotal,
        created_at: new Date().toISOString(),
        updated_at: new Date().toISOString()
      }));

      const { error: itemsError } = await supabase
        .from('order_items')
        .insert(orderItems);

      if (itemsError) {
        await supabase
          .from('orders')
          .delete()
          .eq('id', order.id);
        throw itemsError;
      }

      const fullOrder = await getOrder(order.id);
      dispatch({ type: ACTIONS.SET_ORDER, payload: fullOrder });
      return fullOrder;

    } catch (error) {
      console.error('Error creating order:', error);
      throw error;
    }
  }, [cartItems, dispatch, getOrder]);

  const processCheckout = useCallback(async (billingDetails) => {
    if (!orderState) {
      throw new Error('No order exists');
    }
    
    if (!orderState.id) {
      throw new Error('Invalid order: missing ID');
    }
    
    if (orderState.status !== 'pending') {
      throw new Error(`Cannot process order with status: ${orderState.status}`);
    }

    try {
      const order = await updateOrder(orderState.id, {
        billing_email: billingDetails.email,
        billing_name: billingDetails.name,
        billing_address: billingDetails.address,
        status: 'processing'
      });

      dispatch({ type: ACTIONS.SET_ORDER, payload: order });
      await clearCart();
      return order;

    } catch (error) {
      console.error('Error processing checkout:', error);
      throw error;
    }
  }, [orderState, updateOrder, clearCart, dispatch]);

  const cancelOrder = useCallback(async (orderId) => {
    try {
      const order = await updateOrder(orderId, { status: 'cancelled' });
      dispatch({ type: ACTIONS.CLEAR_ORDER });
      return order;
    } catch (error) {
      console.error('Error cancelling order:', error);
      throw error;
    }
  }, [updateOrder, dispatch]);

  return useMemo(() => ({
    fetchActiveOrder,
    createOrder,
    getOrder,
    updateOrder,
    processCheckout,
    cancelOrder
  }), [
    fetchActiveOrder,
    createOrder,
    getOrder,
    updateOrder,
    processCheckout,
    cancelOrder
  ]);
}

export function OrderProvider({ children }) {
  const [state, dispatch] = useReducer(orderReducer, null);
  const { items: cartItems, clearCart } = useCart();
  const { user } = useContext(AuthContext);
  const operations = useOrderOperations(dispatch, cartItems, clearCart, state);

  useEffect(() => {
    let mounted = true;

    const loadActiveOrder = async () => {
      if (!user) return;

      try {
        const order = await operations.fetchActiveOrder(user.id);
        if (mounted && order) {
          console.log('Found active order:', order);
          dispatch({ type: ACTIONS.SET_ORDER, payload: order });
        } else if (mounted) {
          console.log('No active orders found');
          dispatch({ type: ACTIONS.CLEAR_ORDER });
        }
      } catch (error) {
        console.error('Error loading active order:', error);
      }
    };

    loadActiveOrder();

    return () => {
      mounted = false;
    };
  }, [user, operations.fetchActiveOrder]); // Only depend on fetchActiveOrder

  const value = useMemo(() => ({
    order: state,
    ...operations
  }), [state, operations]);

  return <OrderContext.Provider value={value}>{children}</OrderContext.Provider>;
}

export function useOrder() {
  const context = useContext(OrderContext);
  if (!context) {
    throw new Error('useOrder must be used within an OrderProvider');
  }
  return context;
}

export default OrderProvider;