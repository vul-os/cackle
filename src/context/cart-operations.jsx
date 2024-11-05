// cart-operations.js
import { useState } from 'react';
import { supabase } from '@/services/supabaseClient';

export function useCartOperations() {
  const [operationInProgress, setOperationInProgress] = useState(false);

  const transformCartItems = (cartItems) => {
    return cartItems.map(item => ({
      ticket_type_id: item.ticket_type_id,
      ticket_type: item.ticket_type,
      event_id: item.ticket_type.event.id,
      event: item.ticket_type.event,
      quantity: item.quantity,
      unit_price: item.unit_price,
      subtotal: item.subtotal
    }));
  };

  const mergeCartItems = (localItems, dbItems) => {
    // If there are no DB items, just return local items
    if (!dbItems.length) {
      return localItems;
    }

    // If there are no local items, just return DB items
    if (!localItems.length) {
      return dbItems;
    }

    // Create a map of DB items by ticket_type_id
    const dbItemsMap = new Map(
      dbItems.map(item => [item.ticket_type_id, item])
    );

    // Create a map of local items by ticket_type_id
    const localItemsMap = new Map(
      localItems.map(item => [item.ticket_type_id, item])
    );

    const mergedItems = [];

    // Add all DB items first
    dbItems.forEach(dbItem => {
      mergedItems.push(dbItem);
    });

    // Only add local items that don't exist in DB
    localItems.forEach(localItem => {
      if (!dbItemsMap.has(localItem.ticket_type_id)) {
        mergedItems.push(localItem);
      }
    });

    console.log('Cart Operations - Merged items:', {
      localItems,
      dbItems,
      mergedItems,
    });

    return mergedItems;
  };

  const getOrCreateCart = async (profileId) => {
    if (operationInProgress) {
      console.log('Cart Operations - Operation in progress, skipping getOrCreateCart');
      return null;
    }

    try {
      setOperationInProgress(true);
      console.log('Cart Operations - Getting or creating cart for profile:', profileId);

      // First try to find an existing active cart
      const { data: existingCarts, error: fetchError } = await supabase
        .from('carts')
        .select('id')
        .eq('profile_id', profileId)
        .eq('status', 'active')
        .order('created_at', { ascending: false })
        .limit(1);

      if (fetchError) {
        console.error('Cart Operations - Error fetching existing cart:', fetchError);
        throw fetchError;
      }

      if (existingCarts?.length > 0) {
        console.log('Cart Operations - Found existing cart:', existingCarts[0].id);
        return existingCarts[0].id;
      }

      // If no existing cart, create a new one
      const { data: newCart, error: createError } = await supabase
        .from('carts')
        .insert({
          profile_id: profileId,
          status: 'active',
          created_at: new Date().toISOString(),
          updated_at: new Date().toISOString()
        })
        .select('id')
        .single();

      if (createError) {
        console.error('Cart Operations - Error creating new cart:', createError);
        throw createError;
      }

      console.log('Cart Operations - Created new cart:', newCart.id);
      return newCart.id;

    } catch (error) {
      console.error('Cart Operations - Error in getOrCreateCart:', error);
      throw error;
    } finally {
      setOperationInProgress(false);
    }
  };

  const updateCartInDb = async (cartId, items) => {
    if (operationInProgress) {
      console.log('Cart Operations - Operation in progress, skipping updateCartInDb');
      return;
    }

    if (!cartId) {
      console.error('Cart Operations - No cart ID provided for update');
      throw new Error('Cart ID is required');
    }

    try {
      setOperationInProgress(true);
      console.log('Cart Operations - Updating cart in DB:', { cartId, items });

      // First, delete all existing items for this cart
      const { error: deleteError } = await supabase
        .from('cart_items')
        .delete()
        .eq('cart_id', cartId);

      if (deleteError) {
        console.error('Cart Operations - Error deleting existing items:', deleteError);
        throw deleteError;
      }

      // If there are no new items, we're done
      if (!items.length) {
        console.log('Cart Operations - No new items to insert');
        return;
      }

      // Prepare items for insertion
      const itemsToInsert = items.map(item => ({
        cart_id: cartId,
        ticket_type_id: item.ticket_type_id,
        quantity: item.quantity,
        unit_price: item.unit_price,
        subtotal: item.subtotal,
        created_at: new Date().toISOString(),
        updated_at: new Date().toISOString()
      }));

      // Insert new items
      const { error: insertError } = await supabase
        .from('cart_items')
        .insert(itemsToInsert);

      if (insertError) {
        console.error('Cart Operations - Error inserting new items:', insertError);
        throw insertError;
      }

      // Update cart timestamp
      const { error: updateError } = await supabase
        .from('carts')
        .update({ updated_at: new Date().toISOString() })
        .eq('id', cartId);

      if (updateError) {
        console.error('Cart Operations - Error updating cart timestamp:', updateError);
        throw updateError;
      }

      console.log('Cart Operations - Successfully updated cart in DB');

    } catch (error) {
      console.error('Cart Operations - Error in updateCartInDb:', error);
      throw error;
    } finally {
      setOperationInProgress(false);
    }
  };

  return {
    transformCartItems,
    mergeCartItems,
    getOrCreateCart,
    updateCartInDb,
    operationInProgress
  };
}