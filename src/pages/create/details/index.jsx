import React, { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { Card, CardHeader, CardTitle, CardDescription } from '@/components/ui/card';
import { Building2, FileText, Wallet, ShieldCheck } from 'lucide-react';
import { toast } from '@/components/ui/use-toast';
import { supabase } from '@/services/supabaseClient';
import { OrganizationFormSections } from './form';
import { ProgressSteps, HelpSection, SuccessOverlay } from './helper-components';

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
      <div className="max-w-6xl mx-auto mb-12 text-center">
        <h2 className="text-4xl font-bold text-gray-900 mb-6">
          Let's Get Your Organization Verified! 🔐
        </h2>
        <p className="text-xl text-gray-600 max-w-2xl mx-auto">
          We need a few more details to ensure secure transactions and verify your organization's identity.
          This helps us maintain trust and safety in our community.
        </p>
      </div>

      <ProgressSteps />

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

            <OrganizationFormSections 
              form={form}
              setForm={setForm}
              files={files}
              setFiles={setFiles}
              status={status}
            />
          </form>

          <SuccessOverlay status={status} navigate={navigate} />
        </Card>

        <HelpSection />
      </div>
    </div>
  );
};

export default OrganizationDetailsForm;