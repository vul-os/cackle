import { createClient } from 'jsr:@supabase/supabase-js@2'

export const corsHeaders = {
  'Access-Control-Allow-Origin': '*',
  'Access-Control-Allow-Headers': 'authorization, x-client-info, apikey, content-type',
}

interface Bank {
  name: string
  slug: string
  code: string
  longcode: string
  gateway: null | string
  pay_with_bank: boolean
  active: boolean
  is_deleted: boolean
  country: string
  currency: string
  type: string
  id: number
  createdAt: string
  updatedAt: string
}

interface PaystackResponse {
  status: boolean
  message: string
  data: Bank[]
}

Deno.serve(async (req) => {
  // Handle CORS preflight requests
  if (req.method === 'OPTIONS') {
    return new Response('ok', { headers: corsHeaders })
  }

  try {
    const PAYSTACK_SECRET_KEY = Deno.env.get('PAYSTACK_SECRET_KEY')
    
    if (!PAYSTACK_SECRET_KEY) {
      return new Response(
        JSON.stringify({ error: 'PAYSTACK_SECRET_KEY not configured' }),
        {
          status: 500,
          headers: { ...corsHeaders, 'Content-Type': 'application/json' },
        }
      )
    }

    const response = await fetch('https://api.paystack.co/bank?currency=ZAR', {
      method: 'GET',
      headers: {
        'Authorization': `Bearer ${PAYSTACK_SECRET_KEY}`,
        'Content-Type': 'application/json',
      },
    })

    if (!response.ok) {
      return new Response(
        JSON.stringify({ error: 'Failed to fetch banks from Paystack' }),
        {
          status: response.status,
          headers: { ...corsHeaders, 'Content-Type': 'application/json' },
        }
      )
    }

    const data: PaystackResponse = await response.json()

    return new Response(
      JSON.stringify({
        status: data.status,
        message: data.message,
        data: data.data.map(bank => ({
          name: bank.name,
          slug: bank.slug,
          code: bank.code,
          longcode: bank.longcode,
          gateway: bank.gateway,
          pay_with_bank: bank.pay_with_bank,
          active: bank.active,
          is_deleted: bank.is_deleted,
          country: bank.country,
          currency: bank.currency,
          type: bank.type,
          id: bank.id,
          createdAt: bank.createdAt,
          updatedAt: bank.updatedAt
        }))
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