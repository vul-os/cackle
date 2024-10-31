import React, { useState, useEffect, useCallback, useMemo } from 'react';
import { supabase } from '@/services/supabaseClient';
import { AuthContext } from './use-auth';
import { jwtDecode } from 'jwt-decode'

export function AuthProvider({ children }) {
  const [user, setUser] = useState(null);
  const [loading, setLoading] = useState(true);
  const [organizations, setOrganizations] = useState([]);
  const [activeOrganization, setActiveOrganization] = useState(null);

  window.addEventListener('unhandledrejection', function(event) {
    console.error('Unhandled rejection (promise: ', event.promise, ', reason: ', event.reason, ').'); 
  });

  const fetchOrganizations = useCallback(async () => {
    if (!user) return; // Don't fetch if no user is logged in
    
    try {
      const { data, error } = await supabase
        .from('organizations')
        .select('*')

      if (error) throw error;
      console.log("data", data)
      setOrganizations(data || []);
      // Set first organization as active if none is selected and there are organizations
      if (data && data.length > 0 && !activeOrganization) {
        setActiveOrganization(data[0]);
      }
    } catch (error) {
      console.error('Error fetching organizations:', error);
    }
  }, [user]); // Only depend on user, not activeOrganization

  const handleAuthStateChange = useCallback((event, session) => {
    console.log('Auth state changed:', event);
    setTimeout(() => {
      if (event === 'SIGNED_IN' || event === 'TOKEN_REFRESHED') {
        if (session?.user) {
          setUser(session.user);
        }
      } else if (event === 'SIGNED_OUT' || event === 'USER_DELETED') {
        setUser(null);
        setOrganizations([]);
        setActiveOrganization(null);
      } else if (event === 'USER_UPDATED') {
        setUser(session?.user ?? null);
      }
    }, 0);
  }, []); // Remove fetchOrganizations from dependencies

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
    fetchOrganizations()
  }, [user])

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
    switchOrganization,
    signUp,
    signIn,
    signInWithGoogle,
    signOut,
    forgotPassword,
    updateUserPassword,
  }), [loading, user, organizations, activeOrganization]);

  return (
    <AuthContext.Provider value={contextValue}>
      {children}
    </AuthContext.Provider>
  );
}

export default AuthProvider;