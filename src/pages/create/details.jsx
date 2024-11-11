import React, { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { Card, CardHeader, CardTitle, CardDescription, CardContent, CardFooter } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group";
import { 
  Building2, 
  Loader2, 
  CheckCircle2, 
  XCircle, 
  Upload, 
  Building, 
  FileText, 
  Wallet,
  ArrowRight,
  ShieldCheck
} from 'lucide-react';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { motion, AnimatePresence } from "framer-motion";
import { supabase } from '@/services/supabaseClient';
import { toast } from '@/components/ui/use-toast';

const BANK_CODES = [
  { code: "001", name: "Standard Bank" },
  { code: "002", name: "ABSA" },
  { code: "003", name: "FNB" },
  { code: "004", name: "Nedbank" },
];

const ACCOUNT_TYPES = [
  { value: "personal", label: "Personal Account" },
  { value: "business", label: "Business Account" },
];

const OrganizationDetailsForm = ({ orgId }) => {
  const [form, setForm] = useState({
    entity_type: 'personal',
    business_name: '',
    registration_number: '',
    first_name: '',
    last_name: '',
    identity_number: '',
    contact_number: '',
    bank_code: '',
    account_number: '',
    account_name: '',
    account_type: 'personal',
  });

  const [files, setFiles] = useState({
    identity_document: null,
    registration_document: null,
  });

  const [status, setStatus] = useState('idle');
  const navigate = useNavigate();

  const handleInputChange = (e) => {
    const { name, value } = e.target;
    setForm(prev => ({ ...prev, [name]: value }));
  };

  const handleFileChange = (e, type) => {
    const file = e.target.files[0];
    if (file) {
      setFiles(prev => ({ ...prev, [type]: file }));
    }
  };

  const uploadFile = async (file, path) => {
    const fileExt = file.name.split('.').pop();
    const fileName = `${Date.now()}.${fileExt}`;
    const fullPath = `${orgId}/${path}/${fileName}`;

    const { error: uploadError } = await supabase.storage
      .from('org_documents')
      .upload(fullPath, file);

    if (uploadError) throw uploadError;
    return fullPath;
  };

  const handleSubmit = async (e) => {
    e.preventDefault();
    if (status !== 'idle') return;
    setStatus('loading');

    try {
      let identity_document_path = null;
      let registration_document_path = null;

      if (files.identity_document) {
        identity_document_path = await uploadFile(
          files.identity_document,
          'identity_documents'
        );
      }

      if (files.registration_document) {
        registration_document_path = await uploadFile(
          files.registration_document,
          'registration_documents'
        );
      }

      const { error } = await supabase
        .from('organization_details')
        .insert({
          org_id: orgId,
          ...form,
          identity_document_path,
          registration_document_path,
        });

      if (error) throw error;

      toast({
        title: "Success!",
        description: "Organization details have been saved.",
      });
      setStatus('success');
    } catch (error) {
      console.error('Error saving organization details:', error);
      toast({
        title: "Error",
        description: "Failed to save organization details. Please try again.",
        variant: "destructive"
      });
      setStatus('error');
      setTimeout(() => setStatus('idle'), 2000);
    }
  };

  return (
    <div className="min-h-screen bg-gradient-to-b from-red-50 to-white p-8">
      {/* Hero Section */}
      <div className="max-w-6xl mx-auto mb-12 text-center">
        <h2 className="text-4xl font-bold text-gray-900 mb-6">
          Let's Get Your Organization Verified! 🔐
        </h2>
        <p className="text-xl text-gray-600 max-w-2xl mx-auto">
          We need a few more details to ensure secure transactions and verify your organization's identity.
          This helps us maintain trust and safety in our community.
        </p>
      </div>

      {/* Progress Steps */}
      <div className="max-w-3xl mx-auto mb-8">
        <div className="flex justify-between items-center">
          <div className="flex flex-col items-center">
            <div className="w-10 h-10 bg-green-500 text-white rounded-full flex items-center justify-center mb-2">
              <Building className="w-5 h-5" />
            </div>
            <span className="text-sm text-gray-600">Organization Created</span>
          </div>
          <div className="flex-1 h-1 mx-4 bg-red-200">
            <div className="h-full w-1/2 bg-red-600"></div>
          </div>
          <div className="flex flex-col items-center">
            <div className="w-10 h-10 bg-red-600 text-white rounded-full flex items-center justify-center mb-2">
              <FileText className="w-5 h-5" />
            </div>
            <span className="text-sm text-gray-600">Verification</span>
          </div>
          <div className="flex-1 h-1 mx-4 bg-gray-200"></div>
          <div className="flex flex-col items-center">
            <div className="w-10 h-10 bg-gray-200 text-gray-400 rounded-full flex items-center justify-center mb-2">
              <Wallet className="w-5 h-5" />
            </div>
            <span className="text-sm text-gray-400">Ready to Transact</span>
          </div>
        </div>
      </div>

      <div className="max-w-3xl mx-auto">
        <Card className="shadow-lg">
          <form onSubmit={handleSubmit}>
            <CardHeader>
              <CardTitle className="flex items-center space-x-2">
                <ShieldCheck className="h-6 w-6 text-red-600" />
                <span>Organization Details</span>
              </CardTitle>
              <CardDescription>
                Please provide accurate information to verify your organization
              </CardDescription>
            </CardHeader>

            <CardContent className="space-y-8">
              {/* Entity Type Selection Card */}
              <Card className="border-2 border-gray-100">
                <CardHeader>
                  <CardTitle className="text-lg">Entity Type</CardTitle>
                </CardHeader>
                <CardContent>
                  <RadioGroup
                    defaultValue={form.entity_type}
                    onValueChange={(value) => setForm(prev => ({ ...prev, entity_type: value }))}
                    className="flex space-x-8"
                  >
                    <div className="flex items-center space-x-2">
                      <RadioGroupItem value="personal" id="personal" />
                      <Label htmlFor="personal" className="font-medium">Personal</Label>
                    </div>
                    <div className="flex items-center space-x-2">
                      <RadioGroupItem value="business" id="business" />
                      <Label htmlFor="business" className="font-medium">Business</Label>
                    </div>
                  </RadioGroup>
                </CardContent>
              </Card>

              {/* Entity Details Card */}
              <Card className="border-2 border-gray-100">
                <CardHeader>
                  <CardTitle className="text-lg">
                    {form.entity_type === 'business' ? 'Business Details' : 'Personal Details'}
                  </CardTitle>
                </CardHeader>
                <CardContent className="space-y-4">
                  {form.entity_type === 'business' ? (
                    <>
                      <div>
                        <Label htmlFor="business_name">Business Name</Label>
                        <Input
                          id="business_name"
                          name="business_name"
                          value={form.business_name}
                          onChange={handleInputChange}
                          className="mt-1"
                          required
                        />
                      </div>
                      <div>
                        <Label htmlFor="registration_number">Registration Number</Label>
                        <Input
                          id="registration_number"
                          name="registration_number"
                          value={form.registration_number}
                          onChange={handleInputChange}
                          className="mt-1"
                          required
                        />
                      </div>
                      <div>
                        <Label htmlFor="registration_document">Registration Document</Label>
                        <Input
                          id="registration_document"
                          type="file"
                          onChange={(e) => handleFileChange(e, 'registration_document')}
                          className="mt-1 cursor-pointer"
                          accept=".pdf,.jpg,.jpeg,.png"
                        />
                        <p className="text-sm text-gray-500 mt-1">Upload company registration document (PDF, JPG, PNG)</p>
                      </div>
                    </>
                  ) : (
                    <>
                      <div className="grid grid-cols-2 gap-4">
                        <div>
                          <Label htmlFor="first_name">First Name</Label>
                          <Input
                            id="first_name"
                            name="first_name"
                            value={form.first_name}
                            onChange={handleInputChange}
                            className="mt-1"
                            required
                          />
                        </div>
                        <div>
                          <Label htmlFor="last_name">Last Name</Label>
                          <Input
                            id="last_name"
                            name="last_name"
                            value={form.last_name}
                            onChange={handleInputChange}
                            className="mt-1"
                            required
                          />
                        </div>
                      </div>
                      <div>
                        <Label htmlFor="identity_number">Identity Number</Label>
                        <Input
                          id="identity_number"
                          name="identity_number"
                          value={form.identity_number}
                          onChange={handleInputChange}
                          className="mt-1"
                          required
                        />
                      </div>
                      <div>
                        <Label htmlFor="identity_document">Identity Document</Label>
                        <Input
                          id="identity_document"
                          type="file"
                          onChange={(e) => handleFileChange(e, 'identity_document')}
                          className="mt-1 cursor-pointer"
                          accept=".pdf,.jpg,.jpeg,.png"
                        />
                        <p className="text-sm text-gray-500 mt-1">Upload identity document (PDF, JPG, PNG)</p>
                      </div>
                    </>
                  )}
                </CardContent>
              </Card>

              {/* Contact Details Card */}
              <Card className="border-2 border-gray-100">
                <CardHeader>
                  <CardTitle className="text-lg">Contact Details</CardTitle>
                </CardHeader>
                <CardContent>
                  <div>
                    <Label htmlFor="contact_number">Contact Number</Label>
                    <Input
                      id="contact_number"
                      name="contact_number"
                      value={form.contact_number}
                      onChange={handleInputChange}
                      className="mt-1"
                      required
                    />
                  </div>
                </CardContent>
              </Card>

              {/* Banking Details Card */}
              <Card className="border-2 border-gray-100">
                <CardHeader>
                  <CardTitle className="text-lg">Banking Details</CardTitle>
                </CardHeader>
                <CardContent className="space-y-4">
                  <div>
                    <Label htmlFor="bank_code">Bank</Label>
                    <Select 
                      value={form.bank_code}
                      onValueChange={(value) => setForm(prev => ({ ...prev, bank_code: value }))}
                    >
                      <SelectTrigger className="mt-1">
                        <SelectValue placeholder="Select your bank" />
                      </SelectTrigger>
                      <SelectContent>
                        {BANK_CODES.map((bank) => (
                          <SelectItem key={bank.code} value={bank.code}>
                            {bank.name}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  </div>

                  <div>
                    <Label htmlFor="account_number">Account Number</Label>
                    <Input
                      id="account_number"
                      name="account_number"
                      value={form.account_number}
                      onChange={handleInputChange}
                      className="mt-1"
                      required
                    />
                  </div>

                  <div>
                    <Label htmlFor="account_name">Account Holder Name</Label>
                    <Input
                      id="account_name"
                      name="account_name"
                      value={form.account_name}
                      onChange={handleInputChange}
                      className="mt-1"
                      required
                    />
                  </div>

                  <div>
                    <Label htmlFor="account_type">Account Type</Label>
                    <Select
                      value={form.account_type}
                      onValueChange={(value) => setForm(prev => ({ ...prev, account_type: value }))}
                    >
                      <SelectTrigger className="mt-1">
                        <SelectValue placeholder="Select account type" />
                      </SelectTrigger>
                      <SelectContent>
                        {ACCOUNT_TYPES.map((type) => (
                          <SelectItem key={type.value} value={type.value}>
                            {type.label}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  </div>
                </CardContent>
              </Card>
            </CardContent>

            <CardFooter className="px-6 pb-6">
              <Button 
                type="submit" 
                className="w-full bg-red-600 hover:bg-red-700 text-white h-12 text-lg"
                disabled={status !== 'idle'}
              >
                <AnimatePresence mode="wait">
                  {status === 'idle' && (
                    <motion.span
                      key="idle"
                      initial={{ opacity: 0 }}
                      animate={{ opacity: 1 }}
                      exit={{ opacity: 0 }}
                      className="flex items-center justify-center space-x-2"
                    >
                      <span>Submit for Verification</span>
                      <ArrowRight className="h-5 w-5 ml-2" />
                    </motion.span>
                  )}
                  {status === 'loading' && (
                    <motion.span
                      key="loading"
                      initial={{ opacity: 0 }}
                      animate={{ opacity: 1 }}
                      exit={{ opacity: 0 }}
                      className="flex items-center justify-center space-x-2"
                    >
                      <Loader2 className="h-5 w-5 animate-spin" />
                      <span>Submitting...</span>
                    </motion.span>
                  )}
                  {status === 'success' && (
                    <motion.span
                      key="success"
                      initial={{ opacity: 0, scale: 0.8 }}
                      animate={{ opacity: 1, scale: 1 }}
                      exit={{ opacity: 0 }}
                      className="flex items-center justify-center space-x-2 text-green-500"
                    >
                      <CheckCircle2 className="h-5 w-5" />
                      <span>Verification Submitted!</span>
                    </motion.span>
                  )}
                  {status === 'error' && (
                    <motion.span
                      key="error"
                      initial={{ opacity: 0, scale: 0.8 }}
                      animate={{ opacity: 1, scale: 1 }}
                      exit={{ opacity: 0 }}
                      className="flex items-center justify-center space-x-2 text-red-500"
                    >
                      <XCircle className="h-5 w-5" />
                      <span>Submission Failed</span>
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
                  className="text-center space-y-4 p-6"
                >
                  <CheckCircle2 className="h-16 w-16 text-green-500 mx-auto" />
                  <h3 className="text-2xl font-semibold text-gray-900">Verification Submitted!</h3>
                  <p className="text-gray-600 max-w-md">
                    We've received your organization details and will review them shortly.
                    You'll receive an email once the verification is complete.
                  </p>
                  <Button 
                    onClick={() => navigate('/admin')}
                    className="bg-red-600 hover:bg-red-700 text-white px-8 py-2"
                  >
                    Go to Dashboard
                  </Button>
                </motion.div>
              </motion.div>
            )}
          </AnimatePresence>
        </Card>

        {/* Help Section */}
        <div className="mt-12 text-center">
          <p className="text-gray-600 mb-4">Need help with verification?</p>
          <div className="grid grid-cols-1 md:grid-cols-3 gap-6">
            <div className="bg-white p-6 rounded-xl shadow-sm text-center hover:shadow-md transition-shadow">
              <FileText className="h-8 w-8 text-red-600 mx-auto mb-3" />
              <h3 className="font-semibold text-gray-900 mb-2">Document Guide</h3>
              <p className="text-sm text-gray-600">Learn about the required documents and formats we accept</p>
            </div>
            
            <div className="bg-white p-6 rounded-xl shadow-sm text-center hover:shadow-md transition-shadow">
              <ShieldCheck className="h-8 w-8 text-red-600 mx-auto mb-3" />
              <h3 className="font-semibold text-gray-900 mb-2">Security & Privacy</h3>
              <p className="text-sm text-gray-600">Your data is encrypted and stored securely</p>
            </div>
            
            <div className="bg-white p-6 rounded-xl shadow-sm text-center hover:shadow-md transition-shadow">
              <Building2 className="h-8 w-8 text-red-600 mx-auto mb-3" />
              <h3 className="font-semibold text-gray-900 mb-2">Verification Process</h3>
              <p className="text-sm text-gray-600">Understanding our verification timeline and steps</p>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
};

export default OrganizationDetailsForm;