import React from 'react';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';

const BillingForm = ({ billingDetails, handleInputChange }) => {
    return (
        <Card>
            <CardHeader>
                <CardTitle>Your details</CardTitle>
            </CardHeader>
            <CardContent className="space-y-4">
                <div className="space-y-2">
                    <Label htmlFor="name">Full name</Label>
                    <Input id="name" name="name" value={billingDetails.name} onChange={handleInputChange} required />
                </div>
                <div className="space-y-2">
                    <Label htmlFor="email">Email</Label>
                    <Input id="email" name="email" type="email" value={billingDetails.email} onChange={handleInputChange} required />
                </div>
            </CardContent>
        </Card>
    );
};

export default BillingForm;
