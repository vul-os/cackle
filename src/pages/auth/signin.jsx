import React, { useState, useContext } from 'react';
import { useNavigate } from 'react-router-dom';
import { AuthContext } from '@/context/use-auth';
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Card, CardContent, CardHeader, CardTitle, CardFooter } from "@/components/ui/card";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Separator } from "@/components/ui/separator";
import { Loader2 } from "lucide-react";
import { useAuthRedirect } from './auth-redirect';

import festivalBackground from "/images/celebback.jpg";

const SignIn = () => {
  const { signIn, signInWithGoogle } = useContext(AuthContext);
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
    } catch (error) {
      console.error('Sign in error:', error);
      setError(error.message);
      setIsLoading(false);
    }
  };

  const handleGoogleSignIn = async () => {
    setError('');
    setIsLoading(true);
    try {
      await signInWithGoogle();
      handleSuccessfulAuth();
    } catch (error) {
      console.error('Google sign in error:', error);
      setError(error.message);
      setIsLoading(false);
    }
  };

  return (
    <div 
      className="flex items-center justify-center min-h-screen bg-cover bg-center bg-no-repeat relative"
      style={{
        backgroundImage: `linear-gradient(rgba(0, 0, 0, 0.6), rgba(0, 0, 0, 0.6)), url(${festivalBackground})`,
      }}
    >
      <Card className="w-full max-w-md bg-white shadow-xl border-0 relative z-10">
        <CardHeader className="space-y-1 bg-gradient-to-r from-red-50 to-red-100 rounded-t-lg pb-8">
          <CardTitle className="text-3xl font-bold text-center text-red-800">
            Welcome Back
          </CardTitle>
          <p className="text-center text-red-600 text-sm">Sign in to your account</p>
        </CardHeader>
        <CardContent className="space-y-6 pt-6">
          {error && (
            <Alert variant="destructive" className="bg-red-50 border-red-200">
              <AlertDescription className="text-red-800">{error}</AlertDescription>
            </Alert>
          )}

          <form onSubmit={handleSubmit} className="space-y-4">
            <div className="space-y-2">
              <label htmlFor="email" className="text-sm font-medium text-gray-700">Email</label>
              <Input
                id="email"
                type="email"
                placeholder="Enter your email"
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                disabled={isLoading}
                required
                className="border-gray-200 focus:border-red-400 focus:ring-red-400"
              />
            </div>
            <div className="space-y-2">
              <label htmlFor="password" className="text-sm font-medium text-gray-700">Password</label>
              <Input
                id="password"
                type="password"
                placeholder="Enter your password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                disabled={isLoading}
                required
                className="border-gray-200 focus:border-red-400 focus:ring-red-400"
              />
            </div>
            <Button 
              type="submit" 
              className="w-full bg-gradient-to-r from-red-500 to-red-600 hover:from-red-600 hover:to-red-700 text-white shadow-md"
              disabled={isLoading}
            >
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

          <div className="relative py-4">
            <div className="absolute inset-0 flex items-center">
              <Separator className="w-full bg-gray-200" />
            </div>
            <div className="relative flex justify-center text-xs uppercase">
              <span className="bg-white px-4 text-gray-500 font-medium">
                Or continue with
              </span>
            </div>
          </div>

          <Button
            variant="outline"
            className="w-full bg-white hover:bg-gray-50 text-gray-700 border border-gray-200 shadow-sm"
            onClick={handleGoogleSignIn}
            disabled={isLoading}
          >
            <svg className="mr-2 h-4 w-4 text-red-500" aria-hidden="true" focusable="false" data-prefix="fab" data-icon="google" role="img" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 488 512">
              <path fill="currentColor" d="M488 261.8C488 403.3 391.1 504 248 504 110.8 504 0 393.2 0 256S110.8 8 248 8c66.8 0 123 24.5 166.3 64.9l-67.5 64.9C258.5 52.6 94.3 116.6 94.3 256c0 86.5 69.1 156.6 153.7 156.6 98.2 0 135-70.4 140.8-106.9H248v-85.3h236.1c2.3 12.7 3.9 24.9 3.9 41.4z"></path>
            </svg>
            Sign in with Google
          </Button>
        </CardContent>
        <CardFooter className="flex flex-col space-y-2 bg-gradient-to-r from-red-50 to-red-100 rounded-b-lg p-6">
          <Button 
            variant="link" 
            className="text-sm text-red-600 hover:text-red-700"
            onClick={() => navigate('/signup')}
            disabled={isLoading}
          >
            Don't have an account? Sign Up
          </Button>
          <Button 
            variant="link" 
            className="text-sm text-red-600 hover:text-red-700"
            onClick={() => navigate('/password-reset')}
            disabled={isLoading}
          >
            Forgot your password?
          </Button>
        </CardFooter>
      </Card>
    </div>
  );
};

export default SignIn;