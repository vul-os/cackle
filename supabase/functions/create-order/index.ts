import { createClient } from 'jsr:@supabase/supabase-js@2'

export const corsHeaders = {
  'Access-Control-Allow-Origin': '*',
  'Access-Control-Allow-Headers': 'authorization, x-client-info, apikey, content-type',
}

interface CreateOrderPayload {
  cartId: string
  billingName: string
  billingEmail: string
  billingAddress: {
    street: string
    city: string
    state?: string
    country: string
    postalCode?: string
  }
}

console.log(`Function "create-order" up and running!`)

Deno.serve(async (req) => {
  // Handle CORS preflight requests
  if (req.method === 'OPTIONS') {
    return new Response('ok', { headers: corsHeaders })
  }

  try {
    // Get authorization header
    const authHeader = req.headers.get('Authorization')
    if (!authHeader) {
      return new Response(
        JSON.stringify({ error: 'No authorization header' }),
        {
          status: 401,
          headers: { ...corsHeaders, 'Content-Type': 'application/json' },
        }
      )
    }

    // Initialize Supabase client with user's JWT
    const supabaseClient = createClient(
      Deno.env.get('SUPABASE_URL') ?? '',
      Deno.env.get('SUPABASE_ANON_KEY') ?? '',
      {
        global: {
          headers: {
            Authorization: authHeader
          }
        }
      }
    )

    // Initialize admin client for fetching additional data
    const supabaseAdminClient = createClient(
        Deno.env.get('SUPABASE_URL') ?? '',
        Deno.env.get('SUPABASE_SERVICE_ROLE_KEY') ?? ''
    )
      
    // Parse request body
    const { cartId, billingName, billingEmail, billingAddress }: CreateOrderPayload = await req.json()

    // Validate required fields
    if (!cartId || !billingName || !billingEmail || !billingAddress) {
      return new Response(
        JSON.stringify({ error: 'Missing required fields' }),
        {
          status: 400,
          headers: { ...corsHeaders, 'Content-Type': 'application/json' },
        }
      )
    }

    // Get cart details first
    const { data: cart, error: cartError } = await supabaseClient
      .from('carts')
      .select(`
        *,
        cart_items(
          *,
          ticket_type:ticket_types(
            *,
            event:events(*)
          )
        )
      `)
      .eq('id', cartId)
      .single()

    if (cartError || !cart) {
      return new Response(
        JSON.stringify({ error: 'Cart not found', details: cartError?.message }),
        {
          status: 404,
          headers: { ...corsHeaders, 'Content-Type': 'application/json' },
        }
      )
    }

    // Calculate total amount
    const cartItems = cart.cart_items as any[]
    const totalAmount = cartItems.reduce((sum, item) => 
      sum + (item.quantity * item.unit_price - (item.discount_amount || 0)), 0
    )

    // Create order
    const { data: order, error: orderError } = await supabaseAdminClient
      .from('orders')
      .insert({
        profile_id: (await supabaseClient.auth.getUser()).data.user?.id,
        organization_id: cartItems[0].ticket_type.event.organization_id,
        status: 'pending',
        total_amount: totalAmount,
        billing_name: billingName,
        billing_email: billingEmail,
        billing_address: billingAddress,
        payment_provider: 'paystack',
        metadata: {
          cart_id: cartId
        }
      })
      .select()
      .single()

    if (orderError) {
      console.error('Order creation error:', orderError)
      return new Response(
        JSON.stringify({ error: 'Failed to create order', details: orderError.message }),
        {
          status: 500,
          headers: { ...corsHeaders, 'Content-Type': 'application/json' },
        }
      )
    }

    // Create order items
    const orderItems = cartItems.map(item => ({
      order_id: order.id,
      organization_id: item.ticket_type.event.organization_id,
      event_id: item.ticket_type.event.id,
      ticket_type_id: item.ticket_type_id,
      quantity: item.quantity,
      unit_price: item.unit_price,
      discount_amount: item.discount_amount || 0,
      subtotal: (item.quantity * item.unit_price) - (item.discount_amount || 0)
    }))

    const { error: itemsError } = await supabaseAdminClient
      .from('order_items')
      .insert(orderItems)

    if (itemsError) {
      console.error('Order items creation error:', itemsError)
      // Rollback order
      await supabaseAdminClient.from('orders').delete().eq('id', order.id)
      return new Response(
        JSON.stringify({ error: 'Failed to create order items', details: itemsError.message }),
        {
          status: 500,
          headers: { ...corsHeaders, 'Content-Type': 'application/json' },
        }
      )
    }

    // Initialize Paystack payment
    const paystackPayload = {
      email: billingEmail,
      amount: Math.round(totalAmount * 100), // Convert to kobo/cents
      reference: order.id,
      callback_url: `${Deno.env.get('FRONTEND_URL')}/payment/verify`,
      metadata: {
        order_id: order.id,
        cart_id: cartId
      }
    }

    const paystackResponse = await fetch('https://api.paystack.co/transaction/initialize', {
      method: 'POST',
      headers: {
        'Authorization': `Bearer ${Deno.env.get('PAYSTACK_SECRET_KEY')}`,
        'Content-Type': 'application/json'
      },
      body: JSON.stringify(paystackPayload)
    })

    if (!paystackResponse.ok) {
      const paystackError = await paystackResponse.json()
      // Rollback order and items
      await supabaseAdminClient.from('orders').delete().eq('id', order.id)
      
      return new Response(
        JSON.stringify({ 
          error: 'Payment initialization failed', 
          details: paystackError.message || JSON.stringify(paystackError) 
        }),
        {
          status: 500,
          headers: { ...corsHeaders, 'Content-Type': 'application/json' },
        }
      )
    }

    const paystackData = await paystackResponse.json()

    // Mark cart as converted
    await supabaseClient
      .from('carts')
      .update({ status: 'converted' })
      .eq('id', cartId)

    return new Response(
      JSON.stringify({
        success: true,
        order_id: order.id,
        authorization_url: paystackData.data.authorization_url,
        access_code: paystackData.data.access_code
      }),
      {
        headers: { ...corsHeaders, 'Content-Type': 'application/json' },
      }
    )

  } catch (error) {
    console.error('Function error:', error)
    return new Response(
      JSON.stringify({ 
        error: 'Internal server error', 
        details: error.message || error.toString() 
      }),
      {
        status: 500,
        headers: { ...corsHeaders, 'Content-Type': 'application/json' },
      }
    )
  }
})