import React, { useEffect, useRef, useState } from 'react';
import { useNavigate, useSearchParams } from 'react-router-dom';
import { motion } from 'framer-motion';
import { useAuth } from '@/context/use-auth';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Card, CardContent, CardHeader, CardTitle, CardFooter } from '@/components/ui/card';
import { Alert, AlertDescription } from '@/components/ui/alert';
import { Loader2, Ticket } from 'lucide-react';
import { useAuthRedirect } from './auth-redirect';
import SsoButtons from './sso-buttons';
import { ssoMessageFor } from './sso';

import festivalBackground from '/images/celebback.jpg';

const SignIn = () => {
    const { signIn, refresh } = useAuth();
    const handleSuccessfulAuth = useAuthRedirect();
    const navigate = useNavigate();
    const [searchParams] = useSearchParams();

    const [email, setEmail] = useState('');
    const [password, setPassword] = useState('');
    const [error, setError] = useState('');
    const [isLoading, setIsLoading] = useState(false);

    // A provider sign-in comes back to this page with ?sso=<reason>. The
    // session, when there is one, arrives as a cookie — never in this URL,
    // because a credential in a query string ends up in access logs, in
    // Referer headers and in browser history.
    //
    // On success we re-read /api/auth/me and then hand off to the SAME
    // redirect logic password sign-in uses, so both routes land the person in
    // the same place. On anything else we show a plain sentence and leave the
    // password form exactly where it was — it is what still works.
    const ssoReason = searchParams.get('sso');
    const ssoHandled = useRef(false);
    const [ssoNotice, setSsoNotice] = useState(() => ssoMessageFor(ssoReason));

    useEffect(() => {
        if (!ssoReason || ssoHandled.current) return;
        ssoHandled.current = true;
        setSsoNotice(ssoMessageFor(ssoReason));
        if (ssoReason !== 'ok') return;
        setIsLoading(true);
        refresh()
            .then((session) => {
                if (session?.user) {
                    handleSuccessfulAuth();
                    return;
                }
                setIsLoading(false);
                setSsoNotice(ssoMessageFor('failed'));
            })
            .catch(() => {
                setIsLoading(false);
                setSsoNotice(ssoMessageFor('failed'));
            });
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [ssoReason]);

    const handleSubmit = async (event) => {
        event.preventDefault();
        setError('');
        setIsLoading(true);
        try {
            await signIn(email, password);
            handleSuccessfulAuth();
        } catch (err) {
            setError(err.message || 'Could not sign in.');
            setIsLoading(false);
        }
    };

    return (
        <div
            className="relative flex min-h-screen items-center justify-center bg-cover bg-center bg-no-repeat p-4"
            style={{ backgroundImage: `linear-gradient(rgba(10,8,10,0.75), rgba(10,8,10,0.85)), url(${festivalBackground})` }}
        >
            <motion.div initial={{ opacity: 0, y: 16 }} animate={{ opacity: 1, y: 0 }} transition={{ duration: 0.4 }} className="relative z-10 w-full max-w-md">
                <Card className="border-white/10 bg-card/95 shadow-2xl backdrop-blur">
                    <CardHeader className="space-y-1 pb-6 text-center">
                        <div className="mx-auto mb-2 flex h-12 w-12 items-center justify-center rounded-2xl bg-primary text-primary-foreground">
                            <Ticket className="h-6 w-6" />
                        </div>
                        <CardTitle className="font-display text-3xl font-bold">Welcome back</CardTitle>
                        <p className="text-sm text-muted-foreground">Sign in to manage your events or tickets</p>
                    </CardHeader>
                    <CardContent className="space-y-6">
                        {error && (
                            <Alert variant="destructive">
                                <AlertDescription>{error}</AlertDescription>
                            </Alert>
                        )}

                        {ssoNotice && !error && (
                            <Alert>
                                <AlertDescription>{ssoNotice}</AlertDescription>
                            </Alert>
                        )}

                        <form onSubmit={handleSubmit} className="space-y-4">
                            <div className="space-y-2">
                                <Label htmlFor="email">Email</Label>
                                <Input
                                    id="email"
                                    type="email"
                                    placeholder="you@example.com"
                                    value={email}
                                    onChange={(e) => setEmail(e.target.value)}
                                    disabled={isLoading}
                                    required
                                />
                            </div>
                            <div className="space-y-2">
                                <Label htmlFor="password">Password</Label>
                                <Input
                                    id="password"
                                    type="password"
                                    placeholder="••••••••"
                                    value={password}
                                    onChange={(e) => setPassword(e.target.value)}
                                    disabled={isLoading}
                                    required
                                />
                            </div>
                            <Button type="submit" size="lg" className="w-full" disabled={isLoading}>
                                {isLoading ? (
                                    <>
                                        <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                                        Signing in...
                                    </>
                                ) : (
                                    'Sign In'
                                )}
                            </Button>
                        </form>

                        {/* Renders nothing unless this box's operator set one
                            up. Below the password form on purpose: the
                            password is the sign-in that works with no
                            internet, and this one cannot. */}
                        <SsoButtons disabled={isLoading} />
                    </CardContent>
                    <CardFooter className="flex flex-col space-y-2 border-t border-border pt-6">
                        <Button variant="link" className="text-sm" onClick={() => navigate('/signup')} disabled={isLoading}>
                            Don&apos;t have an account? Sign up
                        </Button>
                        <Button variant="link" className="text-sm text-muted-foreground" onClick={() => navigate('/password-reset')} disabled={isLoading}>
                            Forgot your password?
                        </Button>
                    </CardFooter>
                </Card>
            </motion.div>
        </div>
    );
};

export default SignIn;
