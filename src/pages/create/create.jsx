import React, { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { Card, CardHeader, CardTitle, CardDescription, CardContent, CardFooter } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Building2, Sparkles, PiggyBank, Users, BarChart3, Loader2, CheckCircle2, XCircle } from 'lucide-react';
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { motion, AnimatePresence } from "framer-motion";
import { supabase } from '@/services/supabaseClient';
import { toast } from '@/components/ui/use-toast';

const createOrganization = async (name, description) => {
  const { data, error } = await supabase
    .rpc('create_organization', {
      name,
      description
    });
  
  if (error) throw error;
  return data;
};

const CreateOrganizationPage = () => {
  const [name, setName] = useState('');
  const [description, setDescription] = useState('');
  const [status, setStatus] = useState('idle');
  const navigate = useNavigate();

  const handleSubmit = async (e) => {
    e.preventDefault();
    
    if (status !== 'idle') return; // Prevent multiple submissions
    
    setStatus('loading');
    
    try {
      await createOrganization(name, description);
      toast({
        title: "Success!",
        description: "Your organization has been created.",
      });
      setStatus('success');
    } catch (error) {
      console.error('Error creating organization:', error);
      toast({
        title: "Error",
        description: "Failed to create organization. Please try again.",
        variant: "destructive"
      });
      setStatus('error');
      // Reset to idle after showing error
      setTimeout(() => {
        setStatus('idle');
      }, 2000);
    }
  };

  const handleNavigateToAdmin = () => {
    navigate('/admin', { replace: true });
  };

  return (
    <div className="min-h-screen bg-gradient-to-b from-red-50 to-white p-8">
      {/* Hero Section */}
      <div className="max-w-6xl mx-auto mb-12 text-center">
        <h2 className="text-4xl font-bold text-gray-900 mb-6">
          Hey there, event maestro! 🎉
        </h2>
        <p className="text-xl text-gray-600 max-w-2xl mx-auto">
          Ready to take your events to the next level? Create your organization on Cackle 
          and let's make something amazing together.
        </p>
      </div>

      {/* Form Card */}
      <Card className="max-w-xl mx-auto relative overflow-hidden">
        <form onSubmit={handleSubmit}>
          <CardHeader>
            <CardTitle className="flex items-center space-x-2">
              <Sparkles className="h-6 w-6 text-red-600" />
              <span>Create Your Organization</span>
            </CardTitle>
            <CardDescription>
              Set up your organization profile to get started with Cackle
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-6">
            <div className="space-y-2">
              <Label htmlFor="name">Organization Name</Label>
              <Input 
                id="name" 
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder="Enter your organization name"
                className="w-full"
                required
                disabled={status !== 'idle'}
              />
            </div>
            
            <div className="space-y-2">
              <Label htmlFor="description">Description</Label>
              <Textarea 
                id="description" 
                value={description}
                onChange={(e) => setDescription(e.target.value)}
                placeholder="Tell us about your organization and what kind of events you create..."
                className="w-full min-h-[120px]"
                disabled={status !== 'idle'}
              />
            </div>
          </CardContent>
          <CardFooter>
            <Button 
              type="submit" 
              className="w-full bg-red-600 hover:bg-red-700 text-white relative"
              disabled={status !== 'idle'}
            >
              <AnimatePresence mode="wait">
                {status === 'idle' && (
                  <motion.span
                    key="idle"
                    initial={{ opacity: 0 }}
                    animate={{ opacity: 1 }}
                    exit={{ opacity: 0 }}
                  >
                    Create Organization
                  </motion.span>
                )}
                {status === 'loading' && (
                  <motion.span
                    key="loading"
                    initial={{ opacity: 0 }}
                    animate={{ opacity: 1 }}
                    exit={{ opacity: 0 }}
                    className="flex items-center space-x-2"
                  >
                    <Loader2 className="h-4 w-4 animate-spin" />
                    <span>Creating...</span>
                  </motion.span>
                )}
                {status === 'success' && (
                  <motion.span
                    key="success"
                    initial={{ opacity: 0, scale: 0.8 }}
                    animate={{ opacity: 1, scale: 1 }}
                    exit={{ opacity: 0 }}
                    className="flex items-center space-x-2 text-green-500"
                  >
                    <CheckCircle2 className="h-4 w-4" />
                    <span>Success!</span>
                  </motion.span>
                )}
                {status === 'error' && (
                  <motion.span
                    key="error"
                    initial={{ opacity: 0, scale: 0.8 }}
                    animate={{ opacity: 1, scale: 1 }}
                    exit={{ opacity: 0 }}
                    className="flex items-center space-x-2 text-red-500"
                  >
                    <XCircle className="h-4 w-4" />
                    <span>Error</span>
                  </motion.span>
                )}
              </AnimatePresence>
            </Button>
          </CardFooter>
        </form>
        
        <AnimatePresence>
          {status === 'success' && (
            <motion.div
              initial={{ opacity: 0 }}
              animate={{ opacity: 1 }}
              exit={{ opacity: 0 }}
              className="absolute inset-0 bg-white/80 backdrop-blur-sm flex items-center justify-center"
            >
              <motion.div
                initial={{ scale: 0 }}
                animate={{ scale: 1 }}
                exit={{ scale: 0 }}
                className="text-center space-y-4"
              >
                <CheckCircle2 className="h-16 w-16 text-green-500 mx-auto" />
                <h3 className="text-xl font-semibold text-gray-900">Organization Created!</h3>
                <p className="text-gray-600">We're excited to have you on board!</p>
                <Button 
                  onClick={handleNavigateToAdmin}
                  className="bg-red-600 hover:bg-red-700 text-white"
                >
                  Let's Get to Know You
                </Button>
              </motion.div>
            </motion.div>
          )}
        </AnimatePresence>
      </Card>

      {/* Features */}
      <div className="max-w-xl mx-auto mt-12">
        <p className="text-center text-gray-600 mb-6">By creating an organization, you'll get access to:</p>
        <div className="grid grid-cols-1 md:grid-cols-3 gap-6">
          <div className="bg-white p-6 rounded-xl shadow-sm text-center hover:shadow-md transition-shadow">
            <PiggyBank className="h-8 w-8 text-red-600 mx-auto mb-3" />
            <h3 className="font-semibold text-gray-900 mb-2">Low Fees</h3>
            <p className="text-sm text-gray-600">Industry-leading low transaction fees to maximize your earnings</p>
          </div>
          
          <div className="bg-white p-6 rounded-xl shadow-sm text-center hover:shadow-md transition-shadow">
            <Users className="h-8 w-8 text-red-600 mx-auto mb-3" />
            <h3 className="font-semibold text-gray-900 mb-2">Team Management</h3>
            <p className="text-sm text-gray-600">Invite and manage your team members with flexible permissions</p>
          </div>
          
          <div className="bg-white p-6 rounded-xl shadow-sm text-center hover:shadow-md transition-shadow">
            <BarChart3 className="h-8 w-8 text-red-600 mx-auto mb-3" />
            <h3 className="font-semibold text-gray-900 mb-2">Quality Reporting</h3>
            <p className="text-sm text-gray-600">Detailed analytics and insights to track your event's success</p>
          </div>
        </div>
      </div>
    </div>
  );
};

export default CreateOrganizationPage;