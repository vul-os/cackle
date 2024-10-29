import React, { useState, useContext } from 'react';
import { useNavigate } from 'react-router-dom';
import { AuthContext } from '../../context/use-auth';
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { useToast } from "@/components/ui/use-toast"

const ForgotPassword = () => {
  const { forgotPassword } = useContext(AuthContext);
  const [email, setEmail] = useState('');
  const navigate = useNavigate();
  const { toast } = useToast()

  const handleSubmit = async (event) => {
    event.preventDefault();
    try {
      const { error } = await forgotPassword(email);
      if (error) throw error;
      toast({
        title: "Reset Email Sent",
        description: "Please check your email for the password reset link.",
        duration: 5000,
      })
      // Optionally, redirect to a confirmation page or back to sign in
      navigate('/login');
    } catch (error) {
      toast({
        title: "Password Reset Failed",
        description: error.message,
        variant: "destructive",
        duration: 5000,
      })
    }
  };

  return (
    <div className="container mx-auto max-w-md mt-10">
      <Card>
        <CardHeader>
          <CardTitle className="text-2xl">Forgot Password</CardTitle>
        </CardHeader>
        <CardContent>
          <form onSubmit={handleSubmit} className="space-y-4">
            <div className="space-y-2">
              <label htmlFor="email" className="text-sm font-medium">Email</label>
              <Input
                id="email"
                type="email"
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                required
              />
            </div>
            <Button type="submit" className="w-full">
              Send Reset Link
            </Button>
          </form>
          <div className="mt-4">
            <Button variant="link" className="w-full" onClick={() => navigate('/login')}>
              Remember your password? Sign In
            </Button>
          </div>
        </CardContent>
      </Card>
    </div>
  );
};

export default ForgotPassword;