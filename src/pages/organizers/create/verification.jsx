import React from 'react';
import { Card, CardHeader, CardTitle, CardContent, CardFooter } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogTrigger } from "@/components/ui/dialog";
import { Alert, AlertTitle, AlertDescription } from "@/components/ui/alert";
import { FileText, Clock, CheckCircle2, ArrowUpCircle, AlertCircle, Building2 } from 'lucide-react';
import { OrganizationFormSections } from '@/pages/organizers/create/details/form';

const VerificationPendingPage = ({ orgId }) => {
  return (
    <div className="min-h-screen bg-gradient-to-b from-red-50 to-white p-8">
      {/* Hero Section */}
      <div className="max-w-6xl mx-auto mb-12 text-center">
        <div className="flex justify-center mb-6">
          <div className="w-16 h-16 bg-red-100 rounded-full flex items-center justify-center">
            <Clock className="h-8 w-8 text-red-600" />
          </div>
        </div>
        <h1 className="text-4xl font-bold text-gray-900 mb-4">
          Verification in Progress
        </h1>
        <p className="text-xl text-gray-600 max-w-2xl mx-auto">
          We're carefully reviewing your documents to ensure everything is in order.
          This typically takes 3-5 working days.
        </p>
      </div>

      {/* Progress Steps */}
      <div className="max-w-3xl mx-auto mb-8">
        <div className="flex justify-between items-center">
          <div className="flex flex-col items-center">
            <div className="w-10 h-10 bg-green-500 text-white rounded-full flex items-center justify-center mb-2">
              <Building2 className="w-5 h-5" />
            </div>
            <span className="text-sm text-gray-600">Details Submitted</span>
          </div>
          <div className="flex-1 h-1 mx-4 bg-red-200">
            <div className="h-full w-1/2 animate-pulse bg-red-600"></div>
          </div>
          <div className="flex flex-col items-center">
            <div className="w-10 h-10 bg-red-600 text-white rounded-full flex items-center justify-center mb-2">
              <Clock className="w-5 h-5 animate-spin" />
            </div>
            <span className="text-sm text-gray-600">Under Review</span>
          </div>
          <div className="flex-1 h-1 mx-4 bg-gray-200"></div>
          <div className="flex flex-col items-center">
            <div className="w-10 h-10 bg-gray-200 text-gray-400 rounded-full flex items-center justify-center mb-2">
              <CheckCircle2 className="w-5 h-5" />
            </div>
            <span className="text-sm text-gray-400">Verified</span>
          </div>
        </div>
      </div>

      <div className="max-w-3xl mx-auto space-y-6">
        {/* Status Card */}
        <Card>
          <CardHeader>
            <CardTitle>Verification Status</CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
            <Alert>
              <Clock className="h-4 w-4" />
              <AlertTitle>Under Review</AlertTitle>
              <AlertDescription>
                Submitted on November 11, 2024 at 10:30 AM
                <br />
                Estimated completion: November 14, 2024
              </AlertDescription>
            </Alert>
          </CardContent>
        </Card>

        {/* Documents Card */}
        <Card>
          <CardHeader>
            <CardTitle>Submitted Documents</CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="grid gap-4">
              <DocumentRow
                title="Identity Document"
                status="reviewing"
                fileName="identity_document.pdf"
              />
              <DocumentRow
                title="Business Registration"
                status="reviewing"
                fileName="registration.pdf"
              />
              <DocumentRow
                title="Bank Verification"
                status="reviewing"
                fileName="bank_details.pdf"
              />
            </div>
          </CardContent>
        </Card>

        {/* Update Information Card */}
        <Card>
          <CardHeader>
            <CardTitle>Need to Update Something?</CardTitle>
          </CardHeader>
          <CardContent>
            <p className="text-gray-600 mb-4">
              You can update your information while we're reviewing your documents. 
              Any changes will be reviewed as part of the verification process.
            </p>
            <div className="grid gap-4">
              <UpdateButton 
                title="Update Organization Details"
                description="Change business or personal information"
                icon={<Building2 className="h-4 w-4" />}
              />
              <UpdateButton 
                title="Update Documents"
                description="Upload new or additional documents"
                icon={<ArrowUpCircle className="h-4 w-4" />}
              />
            </div>
          </CardContent>
        </Card>

        {/* Help Card */}
        <Card>
          <CardHeader>
            <CardTitle>Need Help?</CardTitle>
          </CardHeader>
          <CardContent>
            <p className="text-gray-600 mb-4">
              Our support team is here to help if you have any questions about the verification process.
            </p>
            <div className="flex space-x-4">
              <Button variant="outline" className="flex-1">
                View FAQ
              </Button>
              <Button variant="outline" className="flex-1">
                Contact Support
              </Button>
            </div>
          </CardContent>
        </Card>
      </div>
    </div>
  );
};

const DocumentRow = ({ title, status, fileName }) => (
  <div className="flex items-center justify-between p-4 bg-gray-50 rounded-lg">
    <div className="flex items-center space-x-4">
      <FileText className="h-8 w-8 text-gray-400" />
      <div>
        <p className="font-medium text-gray-900">{title}</p>
        <p className="text-sm text-gray-500">{fileName}</p>
      </div>
    </div>
    <div className="flex items-center space-x-2">
      <span className="px-2 py-1 bg-yellow-100 text-yellow-700 rounded-full text-sm font-medium inline-flex items-center">
        <Clock className="h-3 w-3 mr-1" />
        Reviewing
      </span>
      <Dialog>
        <DialogTrigger asChild>
          <Button variant="ghost" size="sm">Update</Button>
        </DialogTrigger>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Update {title}</DialogTitle>
          </DialogHeader>
          <div className="py-4">
            <Button className="w-full" variant="outline">
              <ArrowUpCircle className="h-4 w-4 mr-2" />
              Choose New File
            </Button>
          </div>
        </DialogContent>
      </Dialog>
    </div>
  </div>
);

const UpdateButton = ({ title, description, icon }) => (
  <Dialog>
    <DialogTrigger asChild>
      <button className="flex items-center justify-between p-4 bg-gray-50 rounded-lg hover:bg-gray-100 transition-colors">
        <div className="flex items-center space-x-4">
          {icon}
          <div className="text-left">
            <p className="font-medium text-gray-900">{title}</p>
            <p className="text-sm text-gray-500">{description}</p>
          </div>
        </div>
        <ArrowUpCircle className="h-5 w-5 text-gray-400" />
      </button>
    </DialogTrigger>
    <DialogContent className="max-w-3xl">
      <DialogHeader>
        <DialogTitle>{title}</DialogTitle>
      </DialogHeader>
      <div className="py-4">
        <OrganizationFormSections 
          form={{}}
          setForm={() => {}}
          files={{}}
          setFiles={() => {}}
          status="idle"
        />
      </div>
    </DialogContent>
  </Dialog>
);

export default VerificationPendingPage;