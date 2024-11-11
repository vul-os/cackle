import React, { useState, useEffect, useContext } from 'react';
import { useSearchParams } from 'react-router-dom';
import { AuthContext } from '@/context/use-auth';
import { supabase } from '@/services/supabaseClient';
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Loader2 } from "lucide-react";

import { useAuthRedirect } from './auth-redirect';

const AcceptInvite = () => {
  const { user, setHasLoadedOrganizations } = useContext(AuthContext);
  const handleSuccessfulAuth = useAuthRedirect();
  const [searchParams] = useSearchParams();
  
  const [inviteToken] = useState(searchParams.get('token'));
  const [inviteData, setInviteData] = useState(null);
  const [loading, setLoading] = useState(true);
  const [accepting, setAccepting] = useState(false);
  const [error, setError] = useState('');

  useEffect(() => {
    const fetchInviteData = async () => {
      if (!inviteToken) {
        setError('No invitation token provided');
        setLoading(false);
        return;
      }
  
      try {
        const { data, error } = await supabase
          .rpc('verify_invitation_by_token', { 
            invite_token: inviteToken 
          });
  
        if (error) throw error;

        const fData = data && data?.length > 0 ? data[0] : null
        if (fData && fData.status !== 'pending') {
          throw new Error('Invalid or expired invitation');
        }
  
        setInviteData({
          email: fData.email,
          role: fData.role,
          status: fData.status,
          expires_at: fData.expires_at,
          organizations: {
            id: fData.organization_id,
            name: fData.organization_name
          },
          profiles: {
            full_name: fData.invited_by_name
          }
        });
      } catch (error) {
        console.error('Error fetching invite:', error);
        setError(error.message);
      } finally {
        setLoading(false);
      }
    };
  
    if (user?.email) {
      fetchInviteData();
    }
  }, [inviteToken, user?.email]);

  const handleAcceptInvite = async () => {
    setAccepting(true);
    try {
      const { error } = await supabase.rpc(
        'accept_organization_invitation',
        { p_invitation_token: inviteToken }
      );

      if (error) throw error;
      
      // Trigger a refresh of organizations by setting hasLoadedOrganizations to false
      setHasLoadedOrganizations(false);
      
      await handleSuccessfulAuth();
    } catch (error) {
      setError(error.message);
      setAccepting(false);
    }
  };

  if (loading) {
    return (
      <div className="flex items-center justify-center h-96">
        <Loader2 className="h-8 w-8 animate-spin" />
      </div>
    );
  }

  return (
    <div className="container mx-auto max-w-md mt-10">
      <Card>
        <CardHeader>
          <CardTitle className="text-2xl">Accept Invitation</CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          {error ? (
            <Alert variant="destructive">
              <AlertDescription>{error}</AlertDescription>
            </Alert>
          ) : inviteData && (
            <>
              <Alert>
                <AlertDescription>
                  You've been invited to join {inviteData.organizations.name} as a {inviteData.role} by {inviteData.profiles.full_name}
                </AlertDescription>
              </Alert>

              <Button 
                className="w-full"
                onClick={handleAcceptInvite}
                disabled={accepting}
              >
                {accepting ? (
                  <>
                    <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                    Accepting Invitation...
                  </>
                ) : (
                  'Accept Invitation'
                )}
              </Button>
            </>
          )}
          
          <Button 
            variant="outline" 
            className="w-full" 
            onClick={() => navigate('/admin')}
          >
            {error ? 'Return Home' : 'Cancel'}
          </Button>
        </CardContent>
      </Card>
    </div>
  );
};

export default AcceptInvite;