import { useEffect, useRef, useState, type ChangeEvent, type FormEvent } from 'react';
import { useNavigate, useSearchParams } from 'react-router-dom';
import { useAuth } from '@/context/use-auth';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert';
import { Loader2, KeyRound, TriangleAlert, Clock } from 'lucide-react';
import { newPasswordError, confirmPasswordError, MIN_PASSWORD_LENGTH } from './validate';
import { AuthShell, FieldError } from './auth-shell';

const RAIL_POINTS = [{ icon: Clock, text: 'This link works once and expires an hour after the operator made it.' }];

interface UpdatePasswordFieldErrors {
    newPassword?: string;
    confirmNewPassword?: string;
}

const UpdatePassword = () => {
    const { updatePassword } = useAuth();
    const [searchParams] = useSearchParams();
    const token = searchParams.get('token');
    const navigate = useNavigate();

    const [newPassword, setNewPassword] = useState('');
    const [confirmNewPassword, setConfirmNewPassword] = useState('');
    const [fieldErrors, setFieldErrors] = useState<UpdatePasswordFieldErrors>({});
    const [error, setError] = useState('');
    const [isLoading, setIsLoading] = useState(false);

    const passwordRef = useRef<HTMLInputElement>(null);
    const confirmRef = useRef<HTMLInputElement>(null);
    const alertRef = useRef<HTMLDivElement>(null);

    useEffect(() => {
        if (error) alertRef.current?.focus();
    }, [error]);

    const clearFieldError = (field: keyof UpdatePasswordFieldErrors) => {
        setFieldErrors((f) => (f[field] ? { ...f, [field]: undefined } : f));
    };

    const validate = (): UpdatePasswordFieldErrors => {
        const next: UpdatePasswordFieldErrors = {};
        const passwordMsg = newPasswordError(newPassword);
        const confirmMsg = passwordMsg ? null : confirmPasswordError(newPassword, confirmNewPassword);
        if (passwordMsg) next.newPassword = passwordMsg;
        if (confirmMsg) next.confirmNewPassword = confirmMsg;
        return next;
    };

    const handleSubmit = async (event: FormEvent<HTMLFormElement>) => {
        event.preventDefault();
        setError('');

        const errors = validate();
        if (Object.keys(errors).length > 0) {
            setFieldErrors(errors);
            (errors.newPassword ? passwordRef : confirmRef).current?.focus();
            return;
        }
        setFieldErrors({});
        setIsLoading(true);
        try {
            // The submit button is disabled whenever there is no token (see
            // the form below), so by the time this fires the search param
            // is guaranteed present — the assertion changes nothing about
            // what runs, only what the compiler already knows from the UI.
            await updatePassword(token!, newPassword);
            // A toast fired the instant before this navigate() would compete
            // with the unmount it is about to cause. The sign-in page reads
            // this instead, and shows it after it has actually mounted.
            navigate('/login', { state: { passwordUpdated: true } });
        } catch (err) {
            setError(err instanceof Error ? err.message : 'Could not update your password.');
            setIsLoading(false);
        }
    };

    return (
        <AuthShell
            eyebrow="Reset password"
            title="Choose a new password."
            description={
                token
                    ? 'This link was minted by the operator running this Cackle server. Set a new password below.'
                    : "This link is missing its reset code, so it can't be used."
            }
            points={RAIL_POINTS}
            formHeading="New password"
        >
            <div className="space-y-5">
                {!token && (
                    <Alert variant="destructive">
                        <TriangleAlert className="h-4 w-4" aria-hidden="true" />
                        <AlertTitle>This link won&apos;t work</AlertTitle>
                        <AlertDescription>
                            It&apos;s missing its reset code — probably cut short when it was sent. Ask whoever runs this Cackle
                            server for a new one; the &ldquo;Forgot password&rdquo; page has the command they need.
                        </AlertDescription>
                    </Alert>
                )}

                {error && (
                    <Alert variant="destructive" ref={alertRef} tabIndex={-1}>
                        <TriangleAlert className="h-4 w-4" aria-hidden="true" />
                        <AlertTitle>Couldn&apos;t update your password</AlertTitle>
                        <AlertDescription>{error}</AlertDescription>
                    </Alert>
                )}

                <form onSubmit={handleSubmit} noValidate className="space-y-4">
                    <div className="space-y-1.5">
                        <Label htmlFor="newPassword">New password</Label>
                        <Input
                            id="newPassword"
                            ref={passwordRef}
                            type="password"
                            autoComplete="new-password"
                            value={newPassword}
                            onChange={(e: ChangeEvent<HTMLInputElement>) => {
                                setNewPassword(e.target.value);
                                clearFieldError('newPassword');
                            }}
                            disabled={isLoading || !token}
                            aria-invalid={Boolean(fieldErrors.newPassword)}
                            aria-describedby={fieldErrors.newPassword ? 'newPassword-error' : 'newPassword-hint'}
                        />
                        {fieldErrors.newPassword ? (
                            <FieldError id="newPassword-error">{fieldErrors.newPassword}</FieldError>
                        ) : (
                            <p id="newPassword-hint" className="mt-1.5 text-xs text-muted-foreground">
                                At least {MIN_PASSWORD_LENGTH} characters.
                            </p>
                        )}
                    </div>
                    <div className="space-y-1.5">
                        <Label htmlFor="confirmNewPassword">Confirm new password</Label>
                        <Input
                            id="confirmNewPassword"
                            ref={confirmRef}
                            type="password"
                            autoComplete="new-password"
                            value={confirmNewPassword}
                            onChange={(e: ChangeEvent<HTMLInputElement>) => {
                                setConfirmNewPassword(e.target.value);
                                clearFieldError('confirmNewPassword');
                            }}
                            disabled={isLoading || !token}
                            aria-invalid={Boolean(fieldErrors.confirmNewPassword)}
                            aria-describedby={fieldErrors.confirmNewPassword ? 'confirmNewPassword-error' : undefined}
                        />
                        <FieldError id="confirmNewPassword-error">{fieldErrors.confirmNewPassword}</FieldError>
                    </div>
                    <Button type="submit" size="lg" className="w-full min-h-11" disabled={isLoading || !token}>
                        {isLoading ? (
                            <>
                                <Loader2 className="h-4 w-4 animate-spin" aria-hidden="true" />
                                Updating…
                            </>
                        ) : (
                            <>
                                <KeyRound className="h-4 w-4" aria-hidden="true" />
                                Update password
                            </>
                        )}
                    </Button>
                </form>

                <div className="border-t border-border pt-2">
                    <Button variant="link" className="w-full text-sm" onClick={() => navigate('/login')} disabled={isLoading}>
                        Back to sign in
                    </Button>
                    {!token && (
                        <Button
                            variant="link"
                            className="w-full text-sm text-muted-foreground"
                            onClick={() => navigate('/password-reset')}
                        >
                            Get a new link
                        </Button>
                    )}
                </div>
            </div>
        </AuthShell>
    );
};

export default UpdatePassword;
