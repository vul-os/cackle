import React, { createContext, useCallback, useContext, useEffect, useMemo, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { auth as authApi, setToken, onUnauthorized } from '@/lib/api';

export const AuthContext = createContext({
    loading: true,
    user: null,
    orgs: [],
    activeOrg: null,
    signUp: async () => {},
    signIn: async () => {},
    signOut: async () => {},
    requestPasswordReset: async () => {},
    updatePassword: async () => {},
    switchOrg: () => {},
    refresh: async () => {},
});

const PROTECTED_PREFIXES = ['/admin', '/checkout', '/orders', '/order/', '/tickets', '/ticket/', '/payment', '/accept-invite'];

export function AuthProvider({ children }) {
    const [user, setUser] = useState(null);
    const [orgs, setOrgs] = useState([]);
    const [activeOrgId, setActiveOrgId] = useState(null);
    const [loading, setLoading] = useState(true);
    const navigate = useNavigate();

    const applySession = useCallback((data) => {
        setUser(data?.user ?? null);
        const nextOrgs = data?.orgs ?? [];
        setOrgs(nextOrgs);
        setActiveOrgId((prev) => (prev && nextOrgs.some((o) => o.id === prev) ? prev : nextOrgs[0]?.id ?? null));
    }, []);

    const refresh = useCallback(async () => {
        try {
            const data = await authApi.me({ skipAuthRedirect: true });
            applySession(data);
            return data;
        } catch {
            setUser(null);
            setOrgs([]);
            setActiveOrgId(null);
            return null;
        }
    }, [applySession]);

    useEffect(() => {
        (async () => {
            setLoading(true);
            await refresh();
            setLoading(false);
        })();
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, []);

    useEffect(
        () =>
            onUnauthorized(() => {
                setToken(null);
                setUser(null);
                setOrgs([]);
                setActiveOrgId(null);
                const path = window.location.pathname;
                if (PROTECTED_PREFIXES.some((p) => path.startsWith(p))) {
                    navigate('/login', { state: { returnTo: path } });
                }
            }),
        [navigate],
    );

    const signUp = useCallback(
        async (email, password, name) => {
            const data = await authApi.signup({ email, password, name });
            setToken(data.token);
            applySession(data);
            return data.user;
        },
        [applySession],
    );

    const signIn = useCallback(
        async (email, password) => {
            const data = await authApi.login({ email, password });
            setToken(data.token);
            applySession(data);
            return data.user;
        },
        [applySession],
    );

    const signOut = useCallback(async () => {
        try {
            await authApi.logout();
        } catch {
            // best-effort — clear local state regardless
        }
        setToken(null);
        setUser(null);
        setOrgs([]);
        setActiveOrgId(null);
    }, []);

    const requestPasswordReset = useCallback(async (email) => {
        await authApi.passwordReset(email);
    }, []);

    const updatePassword = useCallback(async (token, password) => {
        await authApi.passwordUpdate(token, password);
    }, []);

    const switchOrg = useCallback(
        (orgId) => {
            if (orgs.some((o) => o.id === orgId)) setActiveOrgId(orgId);
        },
        [orgs],
    );

    const activeOrg = useMemo(() => orgs.find((o) => o.id === activeOrgId) ?? null, [orgs, activeOrgId]);

    const value = useMemo(
        () => ({
            loading,
            user,
            orgs,
            activeOrg,
            signUp,
            signIn,
            signOut,
            requestPasswordReset,
            updatePassword,
            switchOrg,
            refresh,
        }),
        [loading, user, orgs, activeOrg, signUp, signIn, signOut, requestPasswordReset, updatePassword, switchOrg, refresh],
    );

    return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

export function useAuth() {
    const ctx = useContext(AuthContext);
    if (!ctx) throw new Error('useAuth must be used within an AuthProvider');
    return ctx;
}

export default AuthProvider;
