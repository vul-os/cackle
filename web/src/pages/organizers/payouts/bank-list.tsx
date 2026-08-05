import { useEffect, useState, type ChangeEvent } from 'react';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { Input } from '@/components/ui/input';
import { payoutsApi } from '@/lib/api';
import type { Bank } from '@/lib/api-types';

/** docs/API.md pins `GET /api/banks` to exactly `{name,slug,code,currency,active}`
 * — no other field-name variant is ever sent, live or fallback. */
function normalizeBank(bank: Bank): { code: string; name: string } {
    return { code: bank.code, name: bank.name || bank.code };
}

interface BankSelectProps {
    value?: string | null;
    onChange: (value: string) => void;
    disabled?: boolean;
}

/**
 * Bank selector for the payout bank-account form, backed by GET /api/banks.
 * A failed fetch degrades to a plain text code input rather than blocking
 * the form — organisers who already know their bank's code can still set
 * up payouts.
 */
const BankSelect = ({ value, onChange, disabled }: BankSelectProps) => {
    const [banks, setBanks] = useState<Array<{ code: string; name: string }>>([]);
    const [failed, setFailed] = useState(false);
    const [loading, setLoading] = useState(true);

    useEffect(() => {
        let cancelled = false;
        payoutsApi
            .banks()
            .then((data) => {
                if (cancelled) return;
                const list = data?.banks ?? [];
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
        return (
            <Input
                placeholder="Bank code"
                value={value || ''}
                onChange={(e: ChangeEvent<HTMLInputElement>) => onChange(e.target.value)}
                disabled={disabled}
            />
        );
    }

    return (
        <Select onValueChange={onChange} disabled={disabled || loading} {...(value ? { value } : {})}>
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
