import React, { useState, useEffect, useCallback, useMemo } from 'react';
import { supabase } from '@/services/supabaseClient';
import { AuthContext } from './use-auth';
import { jwtDecode } from 'jwt-decode'

export function AuthProvider({ children }) {
  const [user, setUser] = useState(null);
  const [loading, setLoading] = useState(true);
  const [organizations, setOrganizations] = useState([]);
  const [activeOrganization, setActiveOrganization] = useState(null);
  const [hasLoadedOrganizations, setHasLoadedOrganizations] = useState(false);

  window.addEventListener('unhandledrejection', function(event) {
    console.error('Unhandled rejection (promise: ', event.promise, ', reason: ', event.reason, ').'); 
  });

  const fetchOrganizations = useCallback(async () => {
    if (!user) {
      setHasLoadedOrganizations(true); // Mark as loaded even if user is null
      return;
    }
    
    try {
      const { data, error } = await supabase
      .from('organizations')
      .select(`
          *,
          organization_verifications!left (
              verified
          )
      `)

      if (error) throw error;
      console.log("data", data)
      setOrganizations(data || []);
      // Set first organization as active if none is selected and there are organizations
      if (data && data.length > 0 && !activeOrganization) {
        setActiveOrganization(data[0]);
      }
    } catch (error) {
      console.error('Error fetching organizations:', error);
    } finally {
      setHasLoadedOrganizations(true);
    }
  }, [user, activeOrganization]);

  const handleAuthStateChange = useCallback((event, session) => {
    console.log('Auth state changed:', event);
    setTimeout(() => {
      if (event === 'SIGNED_IN' || event === 'TOKEN_REFRESHED') {
        if (session?.user) {
          setUser(session.user);
          setHasLoadedOrganizations(false); // Reset flag when user signs in
        }
      } else if (event === 'SIGNED_OUT' || event === 'USER_DELETED') {
        setUser(null);
        setOrganizations([]);
        setActiveOrganization(null);
        setHasLoadedOrganizations(true); // Mark as loaded when user signs out
      } else if (event === 'USER_UPDATED') {
        setUser(session?.user ?? null);
      }
    }, 0);
  }, []);

  useEffect(() => {
    const initializeAuth = async () => {
      setLoading(true);
      try {
        const { data: { session }, error } = await supabase.auth.getSession();

        if (error) throw error;
        if (session) {
          setUser(session.user);
        }
      } catch (error) {
        console.error('Error initializing auth:', error);
      } finally {
        setLoading(false);
      }
    };

    initializeAuth();

    const { data: { subscription } } = supabase.auth.onAuthStateChange(handleAuthStateChange);
    return () => {
      subscription.unsubscribe();
    };
  }, [handleAuthStateChange]);

  useEffect(() => {
    if (!hasLoadedOrganizations) {
      fetchOrganizations();
    }
  }, [user, hasLoadedOrganizations, fetchOrganizations]);

  const signUp = async (email, password) => {
    const { data, error } = await supabase.auth.signUp({
      email,
      password,
      options: {
        emailRedirectTo: `${window.location.origin}`,
      },
    });
    if (error) throw error;
    return data;
  };

  const signIn = async (email, password) => {
    const { data, error } = await supabase.auth.signInWithPassword({
      email,
      password
    });
    if (error) throw error;
    return data.user;
  };

  const forgotPassword = async (email) => {
    try {
      const { error } = await supabase.auth.resetPasswordForEmail(email, {
        redirectTo: `${window.location.origin}/update-password`,
      });
      if (error) throw error;
      return { error: null };
    } catch (error) {
      return { error };
    }
  };

  const signInWithGoogle = async () => {
    const { data, error } = await supabase.auth.signInWithOAuth({
      provider: 'google',
      options: {
        redirectTo: `${window.location.origin}/dashboard`,
      }
    });
    if (error) throw error;
    return data;
  };

  const signOut = async () => {
    const { error } = await supabase.auth.signOut();
    if (error) throw error;
  };

  const updateUserPassword = async (new_password) => {
    const { data, error } = await supabase.auth.updateUser({
      password: new_password
    });
    if (error) throw error;
    return data;
  };

  const switchOrganization = (orgId) => {
    const newActiveOrg = organizations.find(org => org.id === orgId);
    if (newActiveOrg) {
      setActiveOrganization(newActiveOrg);
    }
  };

  const contextValue = useMemo(() => ({
    loading,
    user,
    organizations,
    activeOrganization,
    hasLoadedOrganizations,
    setHasLoadedOrganizations,
    switchOrganization,
    signUp,
    signIn,
    signInWithGoogle,
    signOut,
    forgotPassword,
    updateUserPassword,
  }), [loading, user, organizations, activeOrganization, hasLoadedOrganizations]);

  return (
    <AuthContext.Provider value={contextValue}>
      {children}
    </AuthContext.Provider>
  );
}

export default AuthProvider;