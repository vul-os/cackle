import React, { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { motion } from 'framer-motion';
import { useAuth } from '@/context/use-auth';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Card, CardContent, CardHeader, CardTitle, CardFooter } from '@/components/ui/card';
import { Alert, AlertDescription } from '@/components/ui/alert';
import { Loader2, Ticket } from 'lucide-react';
import { useAuthRedirect } from './auth-redirect';

import festivalBackground from '/images/celebback.jpg';

const SignIn = () => {
    const { signIn } = useAuth();
    const handleSuccessfulAuth = useAuthRedirect();
    const navigate = useNavigate();

    const [email, setEmail] = useState('');
    const [password, setPassword] = useState('');
    const [error, setError] = useState('');
    const [isLoading, setIsLoading] = useState(false);

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
                            <Button type="submit" className="w-full" disabled={isLoading}>
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
