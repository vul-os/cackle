import React, { useState, useContext } from 'react';
import { useNavigate } from 'react-router-dom';
import { AuthContext } from '../../context/use-auth';
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Card, CardContent, CardHeader, CardTitle, CardFooter } from "@/components/ui/card"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Separator } from "@/components/ui/separator"
import { Loader2 } from "lucide-react"

import { useAuthRedirect } from './auth-redirect';
import backgroundImage from '/images/celebback.jpg'; // Adjust path as needed

const SignUp = () => {
  const { signUp, signInWithGoogle } = useContext(AuthContext);
  const handleSuccessfulAuth = useAuthRedirect();
  const navigate = useNavigate();

  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [confirmPassword, setConfirmPassword] = useState('');
  const [error, setError] = useState('');
  const [isLoading, setIsLoading] = useState(false);

  const handleSubmit = async (event) => {
    event.preventDefault();
    if (password !== confirmPassword) {
      setError("Passwords don't match");
      return;
    }

    setIsLoading(true);
    setError('');

    try {
      await signUp(email, password);
      handleSuccessfulAuth();
    } catch (error) {
      setError(error.message);
      setIsLoading(false);
    }
  };

  const handleGoogleSignUp = async () => {
    setError('');
    setIsLoading(true);
    try {
      await signInWithGoogle();
      handleSuccessfulAuth();
    } catch (error) {
      setError(error.message);
      setIsLoading(false);
    }
  };

  return (
    <div className="flex items-center justify-center min-h-screen bg-gradient-to-br from-[#980F2E] via-[#CD1B41] to-[#FF476F]"
      style={{
        backgroundImage: `url(${backgroundImage})`,
        backgroundSize: 'cover',
        backgroundPosition: 'center',
        opacity: 0.9,
      }}>
      <div className="absolute inset-0 bg-[url('/api/placeholder/1920/1080')] opacity-5 mix-blend-overlay" />
      <Card className="w-full max-w-md backdrop-blur-sm bg-white/90 shadow-2xl border-0 relative overflow-hidden">
        <div className="absolute inset-0 bg-gradient-to-tr from-[#980F2E]/10 to-transparent pointer-events-none" />
        <CardHeader className="space-y-1 relative">
          <CardTitle className="text-3xl font-bold text-center bg-gradient-to-r from-[#980F2E] to-[#CD1B41] bg-clip-text text-transparent">
            Create an account
          </CardTitle>
        </CardHeader>
        <CardContent className="space-y-6 relative">
          {error && (
            <Alert variant="destructive">
              <AlertDescription>{error}</AlertDescription>
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
                className="border-gray-200 focus:border-[#980F2E] focus:ring-[#980F2E] transition-colors"
              />
            </div>
            <div className="space-y-2">
              <label htmlFor="password" className="text-sm font-medium text-gray-700">Password</label>
              <Input
                id="password"
                type="password"
                placeholder="Create a password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                disabled={isLoading}
                required
                className="border-gray-200 focus:border-[#980F2E] focus:ring-[#980F2E] transition-colors"
              />
            </div>
            <div className="space-y-2">
              <label htmlFor="confirmPassword" className="text-sm font-medium text-gray-700">Confirm Password</label>
              <Input
                id="confirmPassword"
                type="password"
                placeholder="Confirm your password"
                value={confirmPassword}
                onChange={(e) => setConfirmPassword(e.target.value)}
                disabled={isLoading}
                required
                className="border-gray-200 focus:border-[#980F2E] focus:ring-[#980F2E] transition-colors"
              />
            </div>
            <Button 
              type="submit" 
              className="w-full bg-gradient-to-r from-[#980F2E] to-[#CD1B41] hover:from-[#CD1B41] hover:to-[#980F2E] text-white transition-all duration-300 shadow-lg hover:shadow-xl" 
              disabled={isLoading}
            >
              {isLoading ? (
                <>
                  <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                  Creating account...
                </>
              ) : (
                'Sign Up'
              )}
            </Button>
          </form>
          <div className="relative">
            <div className="absolute inset-0 flex items-center">
              <Separator className="bg-gray-300" />
            </div>
            <div className="relative flex justify-center text-xs uppercase">
              <span className="bg-white/90 px-2 text-gray-500">
                Or continue with
              </span>
            </div>
          </div>
          <Button
            variant="outline"
            className="w-full border-2 hover:bg-gray-50 transition-colors duration-300"
            onClick={handleGoogleSignUp}
            disabled={isLoading}
          >
            <svg className="mr-2 h-4 w-4" aria-hidden="true" focusable="false" data-prefix="fab" data-icon="google" role="img" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 488 512">
              <path fill="currentColor" d="M488 261.8C488 403.3 391.1 504 248 504 110.8 504 0 393.2 0 256S110.8 8 248 8c66.8 0 123 24.5 166.3 64.9l-67.5 64.9C258.5 52.6 94.3 116.6 94.3 256c0 86.5 69.1 156.6 153.7 156.6 98.2 0 135-70.4 140.8-106.9H248v-85.3h236.1c2.3 12.7 3.9 24.9 3.9 41.4z"></path>
            </svg>
            Sign up with Google
          </Button>
        </CardContent>
        <CardFooter>
          <Button 
            variant="link" 
            className="w-full text-sm text-[#980F2E] hover:text-[#CD1B41] transition-colors" 
            onClick={() => navigate('/login')}
            disabled={isLoading}
          >
            Already have an account? Sign In
          </Button>
        </CardFooter>
      </Card>
    </div>
  );
};

export default SignUp;
