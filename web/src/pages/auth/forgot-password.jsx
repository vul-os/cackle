import React, { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { motion } from 'framer-motion';
import { useAuth } from '@/context/use-auth';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { toast } from '@/components/ui/use-toast';
import { Loader2, Mail } from 'lucide-react';

import rockfestBg from '/images/rockfest.jpg';

const ForgotPassword = () => {
    const { requestPasswordReset } = useAuth();
    const [email, setEmail] = useState('');
    const [isLoading, setIsLoading] = useState(false);
    const [sent, setSent] = useState(false);
    const navigate = useNavigate();

    const handleSubmit = async (event) => {
        event.preventDefault();
        setIsLoading(true);
        try {
            await requestPasswordReset(email);
            setSent(true);
            toast({ title: 'Reset email sent', description: 'Check your inbox for a reset link.' });
        } catch (err) {
            toast({ title: 'Could not send reset email', description: err.message, variant: 'destructive' });
        } finally {
            setIsLoading(false);
        }
    };

    return (
        <div
            className="relative flex min-h-screen items-center justify-center p-4"
            style={{ backgroundImage: `url(${rockfestBg})`, backgroundSize: 'cover', backgroundPosition: 'center' }}
        >
            <div className="absolute inset-0 bg-black/60 backdrop-blur-sm" />
            <motion.div initial={{ opacity: 0, y: 16 }} animate={{ opacity: 1, y: 0 }} transition={{ duration: 0.4 }} className="relative z-10 w-full max-w-md">
                <Card className="border-white/10 bg-card/95 shadow-2xl backdrop-blur">
                    <CardHeader className="text-center">
                        <div className="mx-auto mb-2 flex h-12 w-12 items-center justify-center rounded-2xl bg-primary text-primary-foreground">
                            <Mail className="h-6 w-6" />
                        </div>
                        <CardTitle className="font-display text-2xl font-bold">Forgot password</CardTitle>
                    </CardHeader>
                    <CardContent>
                        {sent ? (
                            <div className="space-y-4 text-center">
                                <p className="text-sm text-muted-foreground">
                                    If an account exists for <span className="font-medium text-foreground">{email}</span>, a reset link is on its way.
                                </p>
                                <Button variant="outline" className="w-full" onClick={() => navigate('/login')}>
                                    Back to sign in
                                </Button>
                            </div>
                        ) : (
                            <form onSubmit={handleSubmit} className="space-y-4">
                                <div className="space-y-2">
                                    <Label htmlFor="email">Email</Label>
                                    <Input id="email" type="email" value={email} onChange={(e) => setEmail(e.target.value)} disabled={isLoading} required />
                                </div>
                                <Button type="submit" className="w-full" disabled={isLoading}>
                                    {isLoading ? (
                                        <>
                                            <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                                            Sending reset link...
                                        </>
                                    ) : (
                                        'Send reset link'
                                    )}
                                </Button>
                                <Button variant="link" className="w-full text-sm" onClick={() => navigate('/login')} disabled={isLoading} type="button">
                                    Remember your password? Sign in
                                </Button>
                            </form>
                        )}
                    </CardContent>
                </Card>
            </motion.div>
        </div>
    );
};

export default ForgotPassword;
