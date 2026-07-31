import React, { useEffect, useRef, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { useAuth } from '@/context/use-auth';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert';
import { Loader2, UserPlus, TriangleAlert, Ticket, ShieldCheck } from 'lucide-react';
import { useAuthRedirect } from './auth-redirect';
import SsoButtons from './sso-buttons';
import { emailError as checkEmail, requiredError, newPasswordError, confirmPasswordError, MIN_PASSWORD_LENGTH } from './validate';
import { AuthShell, FieldError } from './auth-shell';
import { NO_FEE_STATEMENT } from '@/components/honesty/claims';

const RAIL_POINTS = [
    { icon: Ticket, text: NO_FEE_STATEMENT },
    { icon: ShieldCheck, text: 'Your box, your data. Nothing here phones home.' },
];

const SignUp = () => {
    const { signUp } = useAuth();
    const handleSuccessfulAuth = useAuthRedirect();
    const navigate = useNavigate();

    const [name, setName] = useState('');
    const [email, setEmail] = useState('');
    const [password, setPassword] = useState('');
    const [confirmPassword, setConfirmPassword] = useState('');
    const [fieldErrors, setFieldErrors] = useState({});
    const [error, setError] = useState('');
    const [isLoading, setIsLoading] = useState(false);

    const nameRef = useRef(null);
    const emailRef = useRef(null);
    const passwordRef = useRef(null);
    const confirmRef = useRef(null);
    const alertRef = useRef(null);
    const fieldRefs = { name: nameRef, email: emailRef, password: passwordRef, confirmPassword: confirmRef };

    useEffect(() => {
        if (error) alertRef.current?.focus();
    }, [error]);

    const clearFieldError = (field) => {
        setFieldErrors((f) => (f[field] ? { ...f, [field]: undefined } : f));
    };

    const validate = () => {
        const next = {};
        const nameMsg = requiredError(name, 'your name');
        const emailMsg = checkEmail(email);
        const passwordMsg = newPasswordError(password);
        const confirmMsg = passwordMsg ? null : confirmPasswordError(password, confirmPassword);
        if (nameMsg) next.name = nameMsg;
        if (emailMsg) next.email = emailMsg;
        if (passwordMsg) next.password = passwordMsg;
        if (confirmMsg) next.confirmPassword = confirmMsg;
        return next;
    };

    // A server-side rejection still names which of these three things was
    // wrong (see internal/httpapi/auth_handlers.go's handleSignup): an email
    // already in use, a malformed email or a password under the floor. That
    // is a field-level error even though it only surfaces after a round
    // trip, so it gets the same treatment as one caught locally — routed to
    // the field it is actually about, not flattened into one banner every
    // time.
    const fieldForServerError = (message) => {
        if (/email/i.test(message)) return 'email';
        if (/password/i.test(message)) return 'password';
        return null;
    };

    const handleSubmit = async (event) => {
        event.preventDefault();
        setError('');

        const errors = validate();
        if (Object.keys(errors).length > 0) {
            setFieldErrors(errors);
            const firstInvalid = ['name', 'email', 'password', 'confirmPassword'].find((f) => errors[f]);
            fieldRefs[firstInvalid]?.current?.focus();
            return;
        }
        setFieldErrors({});
        setIsLoading(true);
        try {
            await signUp(email, password, name);
            handleSuccessfulAuth();
        } catch (err) {
            const message = err.message || 'Could not create your account.';
            const field = fieldForServerError(message);
            if (field) {
                setFieldErrors({ [field]: message });
                fieldRefs[field]?.current?.focus();
            } else {
                setError(message);
            }
            setIsLoading(false);
        }
    };

    return (
        <AuthShell
            eyebrow="Create account"
            title="Get started."
            description="One account: buy tickets to other events, or open the console and run your own."
            points={RAIL_POINTS}
            formHeading="Create your account"
            formSubheading="Takes under a minute."
        >
            <div className="space-y-5">
                {error && (
                    <Alert variant="destructive" ref={alertRef} tabIndex={-1}>
                        <TriangleAlert className="h-4 w-4" aria-hidden="true" />
                        <AlertTitle>Couldn&apos;t create your account</AlertTitle>
                        <AlertDescription>{error}</AlertDescription>
                    </Alert>
                )}

                <form onSubmit={handleSubmit} noValidate className="space-y-4">
                    <div className="space-y-1.5">
                        <Label htmlFor="name">Name</Label>
                        <Input
                            id="name"
                            ref={nameRef}
                            autoComplete="name"
                            value={name}
                            onChange={(e) => {
                                setName(e.target.value);
                                clearFieldError('name');
                            }}
                            disabled={isLoading}
                            aria-invalid={Boolean(fieldErrors.name)}
                            aria-describedby={fieldErrors.name ? 'name-error' : undefined}
                            className="min-h-11"
                        />
                        <FieldError id="name-error">{fieldErrors.name}</FieldError>
                    </div>
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
                                clearFieldError('email');
                            }}
                            disabled={isLoading}
                            aria-invalid={Boolean(fieldErrors.email)}
                            aria-describedby={fieldErrors.email ? 'email-error' : undefined}
                            className="min-h-11"
                        />
                        <FieldError id="email-error">{fieldErrors.email}</FieldError>
                    </div>
                    <div className="space-y-1.5">
                        <Label htmlFor="password">Password</Label>
                        <Input
                            id="password"
                            ref={passwordRef}
                            type="password"
                            autoComplete="new-password"
                            value={password}
                            onChange={(e) => {
                                setPassword(e.target.value);
                                clearFieldError('password');
                            }}
                            disabled={isLoading}
                            aria-invalid={Boolean(fieldErrors.password)}
                            aria-describedby={fieldErrors.password ? 'password-error' : 'password-hint'}
                            className="min-h-11"
                        />
                        {fieldErrors.password ? (
                            <FieldError id="password-error">{fieldErrors.password}</FieldError>
                        ) : (
                            <p id="password-hint" className="mt-1.5 text-xs text-muted-foreground">
                                At least {MIN_PASSWORD_LENGTH} characters.
                            </p>
                        )}
                    </div>
                    <div className="space-y-1.5">
                        <Label htmlFor="confirmPassword">Confirm password</Label>
                        <Input
                            id="confirmPassword"
                            ref={confirmRef}
                            type="password"
                            autoComplete="new-password"
                            value={confirmPassword}
                            onChange={(e) => {
                                setConfirmPassword(e.target.value);
                                clearFieldError('confirmPassword');
                            }}
                            disabled={isLoading}
                            aria-invalid={Boolean(fieldErrors.confirmPassword)}
                            aria-describedby={fieldErrors.confirmPassword ? 'confirmPassword-error' : undefined}
                            className="min-h-11"
                        />
                        <FieldError id="confirmPassword-error">{fieldErrors.confirmPassword}</FieldError>
                    </div>
                    <Button type="submit" size="lg" className="w-full min-h-11" disabled={isLoading}>
                        {isLoading ? (
                            <>
                                <Loader2 className="h-4 w-4 animate-spin" aria-hidden="true" />
                                Creating your account…
                            </>
                        ) : (
                            <>
                                <UserPlus className="h-4 w-4" aria-hidden="true" />
                                Sign up
                            </>
                        )}
                    </Button>
                </form>

                {/* Renders nothing unless this box's operator set a provider
                    up. A provider sign-in creates the account on first use,
                    so it belongs here too — below the form, for the same
                    reason it is below the form on the sign-in page. */}
                <SsoButtons disabled={isLoading} />

                <div className="border-t border-border pt-2">
                    <Button
                        variant="link"
                        className="min-h-11 w-full text-sm"
                        onClick={() => navigate('/login')}
                        disabled={isLoading}
                    >
                        Already have an account? Sign in
                    </Button>
                </div>
            </div>
        </AuthShell>
    );
};

export default SignUp;
