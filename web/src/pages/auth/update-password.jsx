import React, { useState } from 'react';
import { useNavigate, useSearchParams } from 'react-router-dom';
import { useAuth } from '@/context/use-auth';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Alert, AlertDescription } from '@/components/ui/alert';
import { toast } from '@/components/ui/use-toast';
import { Loader2 } from 'lucide-react';

const UpdatePassword = () => {
    const { updatePassword } = useAuth();
    const [searchParams] = useSearchParams();
    const token = searchParams.get('token');

    const [newPassword, setNewPassword] = useState('');
    const [confirmNewPassword, setConfirmNewPassword] = useState('');
    const [isLoading, setIsLoading] = useState(false);
    const navigate = useNavigate();

    const handleSubmit = async (event) => {
        event.preventDefault();
        if (newPassword !== confirmNewPassword) {
            toast({ title: 'Password mismatch', description: 'New password and confirmation do not match.', variant: 'destructive' });
            return;
        }
        setIsLoading(true);
        try {
            await updatePassword(token, newPassword);
            toast({ title: 'Password updated', description: 'Your password has been changed. Sign in with your new password.' });
            navigate('/login');
        } catch (err) {
            toast({ title: 'Update failed', description: err.message, variant: 'destructive' });
        } finally {
            setIsLoading(false);
        }
    };

    return (
        <div className="container mx-auto mt-10 max-w-md">
            <Card>
                <CardHeader>
                    <CardTitle className="text-2xl">Update password</CardTitle>
                </CardHeader>
                <CardContent>
                    {!token && (
                        <Alert variant="destructive" className="mb-4">
                            <AlertDescription>This link is missing its reset token. Request a new one from the sign-in page.</AlertDescription>
                        </Alert>
                    )}
                    <form onSubmit={handleSubmit} className="space-y-4">
                        <div className="space-y-2">
                            <Label htmlFor="newPassword">New password</Label>
                            <Input
                                id="newPassword"
                                type="password"
                                value={newPassword}
                                onChange={(e) => setNewPassword(e.target.value)}
                                disabled={isLoading}
                                required
                            />
                        </div>
                        <div className="space-y-2">
                            <Label htmlFor="confirmNewPassword">Confirm new password</Label>
                            <Input
                                id="confirmNewPassword"
                                type="password"
                                value={confirmNewPassword}
                                onChange={(e) => setConfirmNewPassword(e.target.value)}
                                disabled={isLoading}
                                required
                            />
                        </div>
                        <Button type="submit" className="w-full" disabled={isLoading || !token}>
                            {isLoading ? (
                                <>
                                    <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                                    Updating...
                                </>
                            ) : (
                                'Update password'
                            )}
                        </Button>
                    </form>
                    <div className="mt-4">
                        <Button variant="link" className="w-full" onClick={() => navigate('/login')} disabled={isLoading}>
                            Back to sign in
                        </Button>
                    </div>
                </CardContent>
            </Card>
        </div>
    );
};

export default UpdatePassword;
