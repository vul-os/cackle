import { useCallback } from 'react';
import { supabase } from '@/services/supabaseClient';
import { ACTIONS } from './order-context';

export function useOrderOperations(dispatch, cartItems, clearCart, orderState) {  // Added orderState parameter
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
  }, [cartItems, dispatch]);

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
    dispatch({ type: ACTIONS.SET_ORDER, payload: order });
    return order;
  }, [dispatch]);

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
    dispatch({ type: ACTIONS.SET_ORDER, payload: updatedOrder });
    return updatedOrder;
  }, [dispatch]);

  const processCheckout = useCallback(async (billingDetails) => {
    if (!orderState) {  // Using orderState instead of state
      throw new Error('No order exists');
    }
    
    if (!orderState.id) {  // Using orderState instead of state
      throw new Error('Invalid order: missing ID');
    }
    
    if (orderState.status !== 'pending') {  // Using orderState instead of state
      throw new Error(`Cannot process order with status: ${orderState.status}`);
    }

    try {
      const order = await updateOrder(orderState.id, {  // Using orderState instead of state
        billing_email: billingDetails.email,
        billing_name: billingDetails.name,
        billing_address: billingDetails.address,
        status: 'processing'
      });

      await clearCart();
      return order;

    } catch (error) {
      console.error('Error processing checkout:', error);
      throw error;
    }
  }, [orderState, updateOrder, clearCart]);  // Added orderState to dependencies

  const cancelOrder = useCallback(async (orderId) => {
    try {
      await updateOrder(orderId, { status: 'cancelled' });
      dispatch({ type: ACTIONS.CLEAR_ORDER });
    } catch (error) {
      console.error('Error cancelling order:', error);
      throw error;
    }
  }, [updateOrder, dispatch]);

  return {
    fetchActiveOrder,
    createOrder,
    getOrder,
    updateOrder,
    processCheckout,
    cancelOrder
  };
}