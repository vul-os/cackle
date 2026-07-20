import React, { useEffect, useState } from 'react';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { Input } from '@/components/ui/input';
import { payoutsApi } from '@/lib/api';

function normalizeBank(bank) {
    // The provider (Paystack) bank list shape isn't pinned down in the
    // contract beyond "provider bank list" — normalise defensively so a few
    // reasonable field-name variants all render correctly.
    const code = bank.code ?? bank.bank_code ?? bank.id ?? bank.slug;
    const name = bank.name ?? bank.label ?? bank.bank_name ?? code;
    return { code, name };
}

/**
 * Bank selector for the payout bank-account form, backed by GET /api/banks.
 * A failed fetch degrades to a plain text code input rather than blocking
 * the form — organisers who already know their bank's code can still set
 * up payouts.
 */
const BankSelect = ({ value, onChange, disabled }) => {
    const [banks, setBanks] = useState([]);
    const [failed, setFailed] = useState(false);
    const [loading, setLoading] = useState(true);

    useEffect(() => {
        let cancelled = false;
        payoutsApi
            .banks()
            .then((data) => {
                if (cancelled) return;
                const list = Array.isArray(data) ? data : (data?.banks ?? []);
                setBanks(list.map(normalizeBank).filter((b) => b.code));
                setLoading(false);
            })
            .catch(() => {
                if (cancelled) return;
                setFailed(true);
                setLoading(false);
            });
        return () => {
            cancelled = true;
        };
    }, []);

    if (failed || (banks.length === 0 && !loading)) {
        return <Input placeholder="Bank code" value={value || ''} onChange={(e) => onChange(e.target.value)} disabled={disabled} />;
    }

    return (
        <Select value={value || undefined} onValueChange={onChange} disabled={disabled || loading}>
            <SelectTrigger>
                <SelectValue placeholder={loading ? 'Loading banks…' : 'Choose your bank'} />
            </SelectTrigger>
            <SelectContent>
                {banks.map((b) => (
                    <SelectItem key={b.code} value={b.code}>
                        {b.name}
                    </SelectItem>
                ))}
            </SelectContent>
        </Select>
    );
};

export default BankSelect;
