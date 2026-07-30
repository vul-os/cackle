import React from 'react';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@/components/ui/card';
import { AlertCircle } from 'lucide-react';
import { TAP_FIELD } from '@/pages/visitor/ui-scale';

/**
 * The two things Cackle needs to issue a ticket, and nothing else. No
 * address, no card fields — the money is handled by whatever rail the
 * organiser configured, which is a separate step.
 *
 * `errors` is a { name?, email? } map from the page. Errors render inline
 * and are wired with aria-invalid / aria-describedby, because a toast that
 * says "missing details" does not tell anybody WHICH field, and on a phone
 * it disappears before they have scrolled back up to look.
 */
const Field = ({ id, label, hint, error, ...inputProps }) => (
    <div className="space-y-2">
        <Label htmlFor={id}>{label}</Label>
        <Input
            id={id}
            name={id}
            className={`${TAP_FIELD} ${error ? 'border-destructive focus-visible:ring-destructive' : ''}`}
            aria-invalid={error ? 'true' : undefined}
            aria-describedby={error ? `${id}-error` : hint ? `${id}-hint` : undefined}
            {...inputProps}
        />
        {error ? (
            <p id={`${id}-error`} className="flex items-center gap-1.5 text-xs font-medium text-destructive">
                <AlertCircle className="h-3.5 w-3.5 shrink-0" aria-hidden="true" />
                {error}
            </p>
        ) : hint ? (
            <p id={`${id}-hint`} className="text-xs text-muted-foreground">
                {hint}
            </p>
        ) : null}
    </div>
);

const BillingForm = ({ billingDetails, handleInputChange, errors = {} }) => (
    <Card>
        <CardHeader>
            <CardTitle>Your details</CardTitle>
            <CardDescription>This is the name that goes on the ticket, and where we send it.</CardDescription>
        </CardHeader>
        <CardContent className="space-y-5">
            <Field
                id="name"
                label="Full name"
                value={billingDetails.name}
                onChange={handleInputChange}
                error={errors.name}
                autoComplete="name"
                required
            />
            <Field
                id="email"
                label="Email"
                type="email"
                inputMode="email"
                value={billingDetails.email}
                onChange={handleInputChange}
                error={errors.email}
                hint="Your tickets appear under My tickets and are sent here."
                autoComplete="email"
                required
            />
        </CardContent>
    </Card>
);

export default BillingForm;
