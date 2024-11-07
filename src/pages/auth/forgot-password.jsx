import React, { useState, useContext } from 'react';
import { useNavigate } from 'react-router-dom';
import { AuthContext } from '../../context/use-auth';
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { useToast } from "@/components/ui/use-toast";
import { Loader2 } from "lucide-react";

// Update the import path to match your project structure
import rockfestBg from '/images/rockfest.jpg';  // Using the @ alias which typically points to src/

const ForgotPassword = () => {
  const { forgotPassword } = useContext(AuthContext);
  const [email, setEmail] = useState('');
  const [isLoading, setIsLoading] = useState(false);
  const navigate = useNavigate();
  const { toast } = useToast();

  const handleSubmit = async (event) => {
    event.preventDefault();
    setIsLoading(true);

    try {
      const { error } = await forgotPassword(email);
      if (error) throw error;

      toast({
        title: "Reset Email Sent",
        description: "Please check your email for the password reset link.",
        duration: 5000,
      });

      navigate('/login');
    } catch (error) {
      toast({
        title: "Password Reset Failed",
        description: error.message,
        variant: "destructive",
        duration: 5000,
      });
    } finally {
      setIsLoading(false);
    }
  };

  return (
    <div 
      className="flex items-center justify-center min-h-screen relative"
      style={{
        backgroundImage: `url(${rockfestBg})`,
        backgroundSize: 'cover',
        backgroundPosition: 'center',
        backgroundRepeat: 'no-repeat'
      }}
    >
      {/* Overlay to ensure text readability */}
      <div className="absolute inset-0 bg-black/50 backdrop-blur-sm" />
      
      <Card className="w-full max-w-md bg-black/20 backdrop-blur-lg shadow-2xl border-red-950/30 relative z-10">
        <CardHeader>
          <CardTitle className="text-2xl font-bold text-center text-red-50">
            Forgot Password
          </CardTitle>
        </CardHeader>
        <CardContent>
          <form onSubmit={handleSubmit} className="space-y-4">
            <div className="space-y-2">
              <label htmlFor="email" className="text-sm font-medium text-red-100">
                Email
              </label>
              <Input
                id="email"
                type="email"
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                disabled={isLoading}
                required
                className="bg-white/10 border-red-900/30 text-red-50 placeholder:text-red-200/50"
              />
            </div>
            <Button 
              type="submit" 
              className="w-full bg-red-800 hover:bg-red-700 text-red-50"
              disabled={isLoading}
            >
              {isLoading ? (
                <>
                  <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                  Sending Reset Link...
                </>
              ) : (
                'Send Reset Link'
              )}
            </Button>
          </form>
          <div className="mt-4">
            <Button
              variant="link"
              className="w-full text-red-200 hover:text-red-100"
              onClick={() => navigate('/login')}
              disabled={isLoading}
            >
              Remember your password? Sign In
            </Button>
          </div>
        </CardContent>
      </Card>
    </div>
  );
};

export default ForgotPassword;