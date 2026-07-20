import React, { useState } from 'react';
import { useNavigate, useSearchParams } from 'react-router-dom';
import { motion } from 'framer-motion';
import { useAuth } from '@/context/use-auth';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Alert, AlertDescription } from '@/components/ui/alert';
import { toast } from '@/components/ui/use-toast';
import { Loader2, KeyRound } from 'lucide-react';

import nightBackground from '/images/night.jpg';

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
        <div
            className="relative flex min-h-screen items-center justify-center bg-cover bg-center bg-no-repeat p-4"
            style={{ backgroundImage: `linear-gradient(rgba(10,8,10,0.75), rgba(10,8,10,0.85)), url(${nightBackground})` }}
        >
            <motion.div initial={{ opacity: 0, y: 16 }} animate={{ opacity: 1, y: 0 }} transition={{ duration: 0.4 }} className="relative z-10 w-full max-w-md">
                <Card className="border-white/10 bg-card/95 shadow-2xl backdrop-blur">
                    <CardHeader className="space-y-1 pb-6 text-center">
                        <div className="mx-auto mb-2 flex h-12 w-12 items-center justify-center rounded-2xl bg-primary text-primary-foreground">
                            <KeyRound className="h-6 w-6" />
                        </div>
                        <CardTitle className="font-display text-2xl font-bold">Update password</CardTitle>
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
            </motion.div>
        </div>
    );
};

export default UpdatePassword;
