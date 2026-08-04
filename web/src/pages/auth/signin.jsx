import React, { useEffect, useRef, useState } from 'react';
import { useLocation, useNavigate, useSearchParams } from 'react-router-dom';
import { useAuth } from '@/context/use-auth';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert';
import { Loader2, LogIn, TriangleAlert, CheckCircle2, ShieldCheck, WifiOff } from 'lucide-react';
import { useAuthRedirect } from './auth-redirect';
import SsoButtons from './sso-buttons';
import { ssoMessageFor } from './sso';
import { emailError as checkEmail, requiredError } from './validate';
import { AuthShell, FieldError } from './auth-shell';

const RAIL_POINTS = [
    {
        icon: WifiOff,
        text: "Password sign-in needs no network to reach — it's what still works when the gate's internet doesn't.",
    },
    { icon: ShieldCheck, text: 'One binary, your box. Nothing here phones home.' },
];

const SignIn = () => {
    const { signIn, refresh } = useAuth();
    const handleSuccessfulAuth = useAuthRedirect();
    const navigate = useNavigate();
    const location = useLocation();
    const [searchParams] = useSearchParams();

    const [email, setEmail] = useState('');
    const [password, setPassword] = useState('');
    const [fieldErrors, setFieldErrors] = useState({});
    const [error, setError] = useState('');
    const [isLoading, setIsLoading] = useState(false);

    const emailRef = useRef(null);
    const passwordRef = useRef(null);
    const alertRef = useRef(null);

    // A password just changed on /update-password lands here with a router
    // state flag rather than a toast — a toast fired the instant before a
    // navigate() unmounts the page that triggered it is exactly the kind of
    // message that is easy to miss, and this is the one page it matters most
    // to see.
    const [passwordUpdated] = useState(() => Boolean(location.state?.passwordUpdated));

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

    // Focus management for the one error that appears after the fields have
    // already passed their own checks: a rejected credential. The alert is
    // conditionally mounted, so the focus call has to wait for the render
    // that puts it in the DOM rather than firing in the same tick as setError.
    useEffect(() => {
        if (error) alertRef.current?.focus();
    }, [error]);

    const validate = () => {
        const next = {};
        const emailMsg = checkEmail(email);
        const passwordMsg = requiredError(password, 'your password');
        if (emailMsg) next.email = emailMsg;
        if (passwordMsg) next.password = passwordMsg;
        return next;
    };

    const handleSubmit = async (event) => {
        event.preventDefault();
        setError('');

        const errors = validate();
        if (Object.keys(errors).length > 0) {
            setFieldErrors(errors);
            (errors.email ? emailRef : passwordRef).current?.focus();
            return;
        }
        setFieldErrors({});
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
        <AuthShell
            eyebrow="Sign in"
            title="Welcome back."
            description="Get back to the door, your events or an order you placed."
            points={RAIL_POINTS}
            formHeading="Sign in"
            formSubheading="Use the email and password you signed up with."
        >
            <div className="space-y-5">
                {passwordUpdated && !error && (
                    <Alert variant="success">
                        <CheckCircle2 className="h-4 w-4" aria-hidden="true" />
                        <AlertDescription>Your password was updated. Sign in with your new one.</AlertDescription>
                    </Alert>
                )}

                {ssoNotice && !error && (
                    <Alert>
                        <TriangleAlert className="h-4 w-4" aria-hidden="true" />
                        <AlertDescription>{ssoNotice}</AlertDescription>
                    </Alert>
                )}

                {error && (
                    <Alert variant="destructive" ref={alertRef} tabIndex={-1}>
                        <TriangleAlert className="h-4 w-4" aria-hidden="true" />
                        <AlertTitle>Couldn&apos;t sign you in</AlertTitle>
                        <AlertDescription>{error}</AlertDescription>
                    </Alert>
                )}

                <form onSubmit={handleSubmit} noValidate className="space-y-4">
                    <div className="space-y-1.5">
                        <Label htmlFor="email">Email</Label>
                        <Input
                            id="email"
                            ref={emailRef}
                            type="email"
                            autoComplete="email"
                            placeholder="you@example.com"
                            value={email}
                            onChange={(e) => {
                                setEmail(e.target.value);
                                if (fieldErrors.email) setFieldErrors((f) => ({ ...f, email: undefined }));
                            }}
                            disabled={isLoading}
                            aria-invalid={Boolean(fieldErrors.email)}
                            aria-describedby={fieldErrors.email ? 'email-error' : undefined}
                        />
                        <FieldError id="email-error">{fieldErrors.email}</FieldError>
                    </div>
                    <div className="space-y-1.5">
                        <Label htmlFor="password">Password</Label>
                        <Input
                            id="password"
                            ref={passwordRef}
                            type="password"
                            autoComplete="current-password"
                            placeholder="••••••••"
                            value={password}
                            onChange={(e) => {
                                setPassword(e.target.value);
                                if (fieldErrors.password) setFieldErrors((f) => ({ ...f, password: undefined }));
                            }}
                            disabled={isLoading}
                            aria-invalid={Boolean(fieldErrors.password)}
                            aria-describedby={fieldErrors.password ? 'password-error' : undefined}
                        />
                        <FieldError id="password-error">{fieldErrors.password}</FieldError>
                    </div>
                    <Button type="submit" size="lg" className="w-full min-h-11" disabled={isLoading}>
                        {isLoading ? (
                            <>
                                <Loader2 className="h-4 w-4 animate-spin" aria-hidden="true" />
                                Signing in…
                            </>
                        ) : (
                            <>
                                <LogIn className="h-4 w-4" aria-hidden="true" />
                                Sign in
                            </>
                        )}
                    </Button>
                </form>

                {/* Renders nothing unless this box's operator set one up. Below
                    the password form on purpose: the password is the sign-in
                    that works with no internet, and this one cannot. */}
                <SsoButtons disabled={isLoading} />

                <div className="flex flex-col border-t border-border pt-2">
                    <Button
                        variant="link"
                        className="w-full text-sm"
                        onClick={() => navigate('/signup')}
                        disabled={isLoading}
                    >
                        Don&apos;t have an account? Sign up
                    </Button>
                    <Button
                        variant="link"
                        className="w-full text-sm text-muted-foreground"
                        onClick={() => navigate('/password-reset')}
                        disabled={isLoading}
                    >
                        Forgot your password?
                    </Button>
                </div>
            </div>
        </AuthShell>
    );
};

export default SignIn;
