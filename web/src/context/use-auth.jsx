import React, { createContext, useCallback, useContext, useEffect, useMemo, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { auth as authApi, orgs as orgsApi, setToken, onUnauthorized } from '@/lib/api';

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
    createOrg: async () => {},
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

    // createOrg is the only way an account gets its FIRST org: signup
    // creates a user row and nothing else, so until this runs every
    // organiser surface has no org to hang off.
    //
    // It re-reads /api/auth/me rather than splicing the response into
    // local state, because that endpoint is the single source of truth for
    // which orgs a user belongs to and at what role — a create is exactly
    // the moment those must not drift apart. If that re-read fails
    // (offline, transient), the new org is merged in locally so the user
    // isn't bounced straight back to "you have no organisation" having
    // just successfully made one; the next refresh reconciles it.
    const createOrg = useCallback(
        async (input) => {
            const data = await orgsApi.create(input);
            const org = data?.org;

            const session = await authApi.me({ skipAuthRedirect: true }).catch(() => null);
            if (session) {
                applySession(session);
            } else if (org?.id) {
                setOrgs((prev) => (prev.some((o) => o.id === org.id) ? prev : [...prev, org]));
            }
            // Set last so it wins over applySession's own choice: the org
            // you just created is the one you want to be looking at.
            if (org?.id) setActiveOrgId(org.id);
            return org;
        },
        [applySession],
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
            createOrg,
            refresh,
        }),
        [loading, user, orgs, activeOrg, signUp, signIn, signOut, requestPasswordReset, updatePassword, switchOrg, createOrg, refresh],
    );

    return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

export function useAuth() {
    const ctx = useContext(AuthContext);
    if (!ctx) throw new Error('useAuth must be used within an AuthProvider');
    return ctx;
}

export default AuthProvider;
