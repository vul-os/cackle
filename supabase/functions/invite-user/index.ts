import { createClient } from 'jsr:@supabase/supabase-js@2'

export const corsHeaders = {
  'Access-Control-Allow-Origin': '*',
  'Access-Control-Allow-Headers': 'authorization, x-client-info, apikey, content-type',
}

console.log(`Function "invite-user" up and running!`)

Deno.serve(async (req) => {
  // Handle CORS preflight requests
  if (req.method === 'OPTIONS') {
    return new Response('ok', { headers: corsHeaders })
  }

  if (req.method !== 'POST') {
    return new Response(JSON.stringify({ error: "Method not allowed" }), {
      status: 405,
      headers: { ...corsHeaders, 'Content-Type': 'application/json' },
    })
  }

  try {
    const RESEND_API_KEY = Deno.env.get('RESEND_API_KEY')
    
    // Get the auth token from the request headers
    const authHeader = req.headers.get('Authorization')
    if (!authHeader) {
      throw new Error('No authorization header')
    }

    const { email, organization_id, role } = await req.json()

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
    // Create invitation using the user's context
    const { data: invitation, error: invitationError } = await supabaseClient.rpc(
      'invite_organization_member',
      {
        p_organization_id: organization_id,
        p_email: email,
        p_role: role
      }
    )

    if (invitationError) {
      console.log(invitationError)
      throw invitationError
    }

    // Initialize admin client for fetching additional data
    const supabaseAdminClient = createClient(
      Deno.env.get('SUPABASE_URL') ?? '',
      Deno.env.get('SUPABASE_SERVICE_ROLE_KEY') ?? ''
    )

    // Fetch organization and profile details
    const { data: orgData, error: orgError } = await supabaseAdminClient
      .from('organizations')
      .select('name')
      .eq('id', organization_id)
      .single()

    if (orgError) {
      console.log("orgerror: ", orgError)
      throw orgError
    } 

    // Get the session or user object
    const token = authHeader.replace('Bearer ', '')
    const { data } = await supabaseClient.auth.getUser(token)
    const user = data.user

    const { data: profileData, error: profileError } = await supabaseAdminClient
      .from('profiles')
      .select('full_name, email')
      .eq('id', user.id)
      .single()

    if (profileError) {
      console.log("profileError: ", profileError)
      throw profileError
    } 
    // Send email using fetch instead of Resend SDK
    const appUrl = Deno.env.get('APP_URL')
    const inviteUrl = `${appUrl}/accept-invite?token=${invitation}`

    const emailResponse = await fetch('https://api.resend.com/emails', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'Authorization': `Bearer ${RESEND_API_KEY}`
      },
      body: JSON.stringify({
        from: 'Your App <info@cackle.co.za>',
        to: [email],
        subject: `Join ${orgData.name} on YourApp`,
        html: `
          <div>
            <h2>You've been invited to join ${orgData.name}</h2>
            <p>Hello,</p>
            <p>${profileData.full_name} (${profileData.email}) has invited you to join ${orgData.name} as a ${role}.</p>
            <p>Click the link below to accept the invitation:</p>
            <p>
              <a href="${inviteUrl}">Accept Invitation</a>
            </p>
            <p>This invitation will expire in 7 days.</p>
          </div>
        `
      })
    })

    if (!emailResponse.ok) {
      const emailError = await emailResponse.json()
      throw new Error(`Failed to send email: ${emailError.message}`)
    }

    return new Response(
      JSON.stringify({ success: true }),
      { 
        headers: { ...corsHeaders, 'Content-Type': 'application/json' },
        status: 200
      }
    )

  } catch (error) {
    return new Response(
      JSON.stringify({ error: error.message }),
      { 
        status: 400,
        headers: { ...corsHeaders, 'Content-Type': 'application/json' }
      }
    )
  }
})