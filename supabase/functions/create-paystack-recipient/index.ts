import { createClient } from 'jsr:@supabase/supabase-js@2'

export const corsHeaders = {
  'Access-Control-Allow-Origin': '*',
  'Access-Control-Allow-Headers': 'authorization, x-client-info, apikey, content-type',
}


interface PaystackRecipient {
  active: boolean
  createdAt: string
  currency: string
  domain: string
  id: number
  integration: number
  name: string
  recipient_code: string
  type: string
  updatedAt: string
  is_deleted: boolean
  details: {
    authorization_code: string | null
    account_number: string
    account_name: string
    bank_code: string
    bank_name: string
  }
}

interface PaystackResponse {
  status: boolean
  message: string
  data: PaystackRecipient
}

Deno.serve(async (req) => {
  if (req.method === 'OPTIONS') {
    return new Response('ok', { headers: corsHeaders })
  }

  try {
    const PAYSTACK_SECRET_KEY = Deno.env.get('PAYSTACK_SECRET_KEY')
    const SUPABASE_URL = Deno.env.get('SUPABASE_URL')
    const SUPABASE_SERVICE_ROLE_KEY = Deno.env.get('SUPABASE_SERVICE_ROLE_KEY')
    
    if (!PAYSTACK_SECRET_KEY || !SUPABASE_URL || !SUPABASE_SERVICE_ROLE_KEY) {
      return new Response(
        JSON.stringify({ error: 'Missing required environment variables' }),
        {
          status: 500,
          headers: { ...corsHeaders, 'Content-Type': 'application/json' },
        }
      )
    }

    // Initialize Supabase client with service role key
    const supabase = createClient(SUPABASE_URL, SUPABASE_SERVICE_ROLE_KEY)

    if (req.method !== 'POST') {
      return new Response(
        JSON.stringify({ error: 'Method not allowed' }),
        {
          status: 405,
          headers: { ...corsHeaders, 'Content-Type': 'application/json' },
        }
      )
    }

    // Get organization_id from request
    const { organization_id } = await req.json()
    if (!organization_id) {
      return new Response(
        JSON.stringify({ error: 'Organization ID is required' }),
        {
          status: 400,
          headers: { ...corsHeaders, 'Content-Type': 'application/json' },
        }
      )
    }

    // Fetch organization details
    const { data: orgDetails, error: fetchError } = await supabase
      .from('organization_details')
      .select('*')
      .eq('organization_id', organization_id)
      .single()

    if (fetchError || !orgDetails) {
      return new Response(
        JSON.stringify({ 
          error: 'Failed to fetch organization details',
          details: fetchError
        }),
        {
          status: 404,
          headers: { ...corsHeaders, 'Content-Type': 'application/json' },
        }
      )
    }

    // Create Paystack recipient using organization details
    const paystackResponse = await fetch('https://api.paystack.co/transferrecipient', {
      method: 'POST',
      headers: {
        'Authorization': `Bearer ${PAYSTACK_SECRET_KEY}`,
        'Content-Type': 'application/json',
      },
      body: JSON.stringify({
        type: 'basa', // Assuming nuban for SA bank accounts
        name: orgDetails.account_name,
        account_number: orgDetails.account_number,
        bank_code: orgDetails.bank_code,
        currency: 'ZAR' // Assuming ZAR as default currency
      })
    })

    if (!paystackResponse.ok) {
      const errorData = await paystackResponse.json()
      return new Response(
        JSON.stringify({ 
          error: 'Failed to create recipient',
          details: errorData
        }),
        {
          status: paystackResponse.status,
          headers: { ...corsHeaders, 'Content-Type': 'application/json' },
        }
      )
    }

    const paystackData: PaystackResponse = await paystackResponse.json()
    
    // Update organization_verifications table
    const { error: updateError } = await supabase
      .from('organization_verifications')
      .upsert({
        organization_id: organization_id,
        paystack_recipient_id: paystackData.data.recipient_code,
        verified: true,
        updated_at: new Date().toISOString()
      }, {
        onConflict: 'organization_id'
      })

    if (updateError) {
      console.error('Database error:', updateError)
      return new Response(
        JSON.stringify({ 
          error: 'Recipient created but failed to update organization verification',
          details: updateError,
          recipient: paystackData.data
        }),
        {
          status: 500,
          headers: { ...corsHeaders, 'Content-Type': 'application/json' },
        }
      )
    }

    return new Response(
      JSON.stringify({
        status: true,
        message: 'Recipient created and organization updated successfully',
        data: {
          recipient: {
            recipient_code: paystackData.data.recipient_code,
            details: paystackData.data.details
          },
          organization: {
            id: organization_id,
            verified: true
          }
        }
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