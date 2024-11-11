import React from 'react';
import { Card, CardHeader, CardTitle, CardContent, CardFooter } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Loader2, CheckCircle2, XCircle, ArrowRight } from 'lucide-react';
import { motion, AnimatePresence } from "framer-motion";

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

export const OrganizationFormSections = ({ form, setForm, files, setFiles, status }) => {
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

  return (
    <>
      <CardContent className="space-y-8">
        <EntityTypeSection form={form} setForm={setForm} />
        <EntityDetailsSection 
          form={form} 
          handleInputChange={handleInputChange}
          handleFileChange={handleFileChange}
        />
        <ContactDetailsSection form={form} handleInputChange={handleInputChange} />
        <BankingDetailsSection form={form} setForm={setForm} handleInputChange={handleInputChange} />
      </CardContent>

      <CardFooter className="px-6 pb-6">
        <SubmitButton status={status} />
      </CardFooter>
    </>
  );
};

const EntityTypeSection = ({ form, setForm }) => (
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
);

const BusinessDetailsFields = ({ form, handleInputChange, handleFileChange }) => (
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
);

const PersonalDetailsFields = ({ form, handleInputChange, handleFileChange }) => (
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
);

const EntityDetailsSection = ({ form, handleInputChange, handleFileChange }) => (
  <Card className="border-2 border-gray-100">
    <CardHeader>
      <CardTitle className="text-lg">
        {form.entity_type === 'business' ? 'Business Details' : 'Personal Details'}
      </CardTitle>
    </CardHeader>
    <CardContent className="space-y-4">
      {form.entity_type === 'business' ? (
        <BusinessDetailsFields 
          form={form}
          handleInputChange={handleInputChange}
          handleFileChange={handleFileChange}
        />
      ) : (
        <PersonalDetailsFields 
          form={form}
          handleInputChange={handleInputChange}
          handleFileChange={handleFileChange}
        />
      )}
    </CardContent>
  </Card>
);

const ContactDetailsSection = ({ form, handleInputChange }) => (
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
);

const BankingDetailsSection = ({ form, setForm, handleInputChange }) => (
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
);

const SubmitButton = ({ status }) => (
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
);