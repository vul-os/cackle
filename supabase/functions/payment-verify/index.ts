import { createClient } from 'jsr:@supabase/supabase-js@2'

export const corsHeaders = {
  'Access-Control-Allow-Origin': '*',
  'Access-Control-Allow-Headers': 'authorization, x-client-info, apikey, content-type',
}

Deno.serve(async (req) => {
  // Handle CORS preflight requests
  if (req.method === 'OPTIONS') {
    return new Response('ok', { headers: corsHeaders })
  }

  try {
    // Get reference from URL params
    const url = new URL(req.url)
    const reference = url.searchParams.get('reference')
    
    if (!reference) {
      return new Response(
        JSON.stringify({ error: 'Reference not provided' }),
        {
          status: 400,
          headers: { ...corsHeaders, 'Content-Type': 'application/json' },
        }
      )
    }

    // Verify payment with Paystack
    const verifyResponse = await fetch(
      `https://api.paystack.co/transaction/verify/${reference}`,
      {
        headers: {
          'Authorization': `Bearer ${Deno.env.get('PAYSTACK_SECRET_KEY')}`,
          'Content-Type': 'application/json'
        }
      }
    )

    if (!verifyResponse.ok) {
      return new Response(
        JSON.stringify({ error: 'Payment verification failed' }),
        {
          status: 400,
          headers: { ...corsHeaders, 'Content-Type': 'application/json' },
        }
      )
    }

    const paymentData = await verifyResponse.json()
    
    // Initialize Supabase admin client
    const supabaseAdmin = createClient(
      Deno.env.get('SUPABASE_URL') ?? '',
      Deno.env.get('SUPABASE_SERVICE_ROLE_KEY') ?? '',
      {
        auth: {
          autoRefreshToken: false,
          persistSession: false
        }
      }
    )

    // Call the database function to update payment status
    const { data: orderId, error: updateError } = await supabaseAdmin.rpc(
      'update_payment_status',
      {
        p_reference: reference,
        p_status: paymentData.data.status,
        p_transaction_id: paymentData.data.id.toString()
      }
    )

    if (updateError) {
      console.error('Payment update error:', updateError)
      return new Response(
        JSON.stringify({ error: 'Failed to update payment status' }),
        {
          status: 500,
          headers: { ...corsHeaders, 'Content-Type': 'application/json' },
        }
      )
    }

    // Return success response with order ID
    return new Response(
      JSON.stringify({
        success: true,
        order_id: orderId,
        status: paymentData.data.status
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