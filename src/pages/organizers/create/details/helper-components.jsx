import React from 'react';
import { Button } from '@/components/ui/button';
import { Building2, FileText, Wallet, Building, ShieldCheck, CheckCircle2 } from 'lucide-react';
import { motion, AnimatePresence } from "framer-motion";

export const ProgressSteps = () => (
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
);

export const HelpSection = () => (
  <div className="mt-12 text-center">
    <p className="text-gray-600 mb-4">Need help with verification?</p>
    <div className="grid grid-cols-1 md:grid-cols-3 gap-6">
      <HelpCard
        icon={<FileText className="h-8 w-8 text-red-600 mx-auto mb-3" />}
        title="Document Guide"
        description="Learn about the required documents and formats we accept"
      />
      <HelpCard
        icon={<ShieldCheck className="h-8 w-8 text-red-600 mx-auto mb-3" />}
        title="Security & Privacy"
        description="Your data is encrypted and stored securely"
      />
      <HelpCard
        icon={<Building2 className="h-8 w-8 text-red-600 mx-auto mb-3" />}
        title="Verification Process"
        description="Understanding our verification timeline and steps"
      />
    </div>
  </div>
);

const HelpCard = ({ icon, title, description }) => (
  <div className="bg-white p-6 rounded-xl shadow-sm text-center hover:shadow-md transition-shadow">
    {icon}
    <h3 className="font-semibold text-gray-900 mb-2">{title}</h3>
    <p className="text-sm text-gray-600">{description}</p>
  </div>
);

export const SuccessOverlay = ({ status, navigate }) => (
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
);