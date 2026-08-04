import { useEffect, useMemo, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { z } from 'zod';
import { zodResolver } from '@hookform/resolvers/zod';
import { useForm } from 'react-hook-form';
import { Building2, ArrowRight, Loader2, ArrowLeft } from 'lucide-react';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Alert, AlertDescription } from '@/components/ui/alert';
import { Form, FormControl, FormDescription, FormField, FormItem, FormLabel, FormMessage } from '@/components/ui/form';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { useAuth } from '@/context/use-auth';
import { currencies as currenciesApi, ApiError } from '@/lib/api';
import type { Currency } from '@/lib/api-types';
import { slugifyOrgName } from './slug';

/** The fallback list carries no `exponent` — only the live server table
 * (internal/money) does — so the picker only ever needs `code`/`name`. */
type CurrencyOption = Pick<Currency, 'code' | 'name'>;

// Fallback only, used if GET /api/currencies fails (offline, transient).
// Cackle has no privileged currency, so the real list is the full ISO-4217
// table from the server — see internal/money.
const FALLBACK_CURRENCIES: CurrencyOption[] = [
    { code: 'USD', name: 'United States Dollar' },
    { code: 'EUR', name: 'Euro' },
    { code: 'GBP', name: 'British Pound Sterling' },
    { code: 'ZAR', name: 'South African Rand' },
];

const createOrgSchema = z.object({
    name: z.string().trim().min(2, 'Give your organisation a name.').max(120, 'Keep the name under 120 characters.'),
    slug: z.string().trim().optional(),
    default_currency: z.string().optional(),
});

const CreateOrgPage = () => {
    const navigate = useNavigate();
    const { orgs, createOrg } = useAuth();
    const [submitError, setSubmitError] = useState<string | null>(null);
    const [submitting, setSubmitting] = useState(false);
    const [currencyOptions, setCurrencyOptions] = useState<CurrencyOption[]>(FALLBACK_CURRENCIES);

    const isFirstOrg = !orgs || orgs.length === 0;

    useEffect(() => {
        let cancelled = false;
        currenciesApi
            .list()
            .then((data) => {
                if (cancelled) return;
                const list = data?.currencies;
                if (Array.isArray(list) && list.length > 0) setCurrencyOptions(list);
            })
            .catch(() => {
                // Keep the fallback — a short currency list beats a broken form.
            });
        return () => {
            cancelled = true;
        };
    }, []);

    const form = useForm<{ name: string; slug?: string; default_currency?: string }>({
        resolver: zodResolver(createOrgSchema),
        defaultValues: { name: '', slug: '', default_currency: '' },
    });

    const nameValue = form.watch('name');
    const slugValue = form.watch('slug');

    // The slug field is left empty unless the organiser edits it, and the
    // preview falls back to what the server would derive from the name.
    // That keeps one rule ("derived from your name unless you say
    // otherwise") instead of two fields silently fighting each other.
    const effectiveSlug = useMemo(() => slugifyOrgName(slugValue) || slugifyOrgName(nameValue), [slugValue, nameValue]);

    const onSubmit = async (values: { name: string; slug?: string; default_currency?: string }) => {
        setSubmitting(true);
        setSubmitError(null);
        try {
            await createOrg({
                name: values.name,
                slug: slugifyOrgName(values.slug) || undefined,
                default_currency: values.default_currency || undefined,
            });
            // Straight to the console, now that there is something behind it.
            // replace: true so Back doesn't land on a form that would now
            // 409 against the org it just created.
            navigate('/admin', { replace: true });
        } catch (err) {
            if (err instanceof ApiError && err.code === 'conflict') {
                form.setError('slug', {
                    type: 'server',
                    message: 'That web address is already taken. Try adding your city or year.',
                });
            } else {
                setSubmitError(err instanceof Error ? err.message : 'Could not create your organisation.');
            }
            setSubmitting(false);
        }
    };

    return (
        <div className="mx-auto max-w-2xl">
            {!isFirstOrg && (
                <Button variant="ghost" size="sm" className="mb-4 -ml-2" onClick={() => navigate('/admin')} disabled={submitting}>
                    <ArrowLeft className="mr-2 h-4 w-4" />
                    Back to dashboard
                </Button>
            )}

            <div className="mb-8 flex items-start gap-4">
                <div className="flex h-12 w-12 shrink-0 items-center justify-center rounded-2xl bg-primary text-primary-foreground">
                    <Building2 className="h-6 w-6" />
                </div>
                <div className="min-w-0">
                    <h1 className="font-display text-2xl font-bold sm:text-3xl">
                        {isFirstOrg ? 'Set up your organisation' : 'Create another organisation'}
                    </h1>
                    <p className="mt-1 text-balance text-muted-foreground">
                        {isFirstOrg
                            ? 'This is the name your events are sold under — your venue, promoter name, church, or club. You can change it later.'
                            : 'A separate organisation keeps its own events, team and payouts. Nothing is shared between them.'}
                    </p>
                </div>
            </div>

            {submitError && (
                <Alert variant="destructive" className="mb-6">
                    <AlertDescription>{submitError}</AlertDescription>
                </Alert>
            )}

            <Card>
                <CardHeader>
                    <CardTitle>Organisation details</CardTitle>
                    <CardDescription>Two things to fill in. The rest can wait.</CardDescription>
                </CardHeader>
                <CardContent>
                    <Form {...form}>
                        <form onSubmit={form.handleSubmit(onSubmit)} className="space-y-6">
                            <FormField
                                control={form.control}
                                name="name"
                                render={({ field }) => (
                                    <FormItem>
                                        <FormLabel>Name</FormLabel>
                                        <FormControl>
                                            <Input {...field} placeholder="e.g. The Old Biscuit Mill" autoFocus disabled={submitting} />
                                        </FormControl>
                                        <FormDescription>Buyers see this on the ticket and at checkout.</FormDescription>
                                        <FormMessage />
                                    </FormItem>
                                )}
                            />

                            <FormField
                                control={form.control}
                                name="slug"
                                render={({ field }) => (
                                    <FormItem>
                                        <FormLabel>Web address (optional)</FormLabel>
                                        <FormControl>
                                            <Input
                                                {...field}
                                                placeholder={slugifyOrgName(nameValue) || 'the-old-biscuit-mill'}
                                                disabled={submitting}
                                                autoCapitalize="none"
                                                autoCorrect="off"
                                                spellCheck={false}
                                            />
                                        </FormControl>
                                        <FormDescription>
                                            {effectiveSlug ? (
                                                <>
                                                    Leave this blank and we&apos;ll use{' '}
                                                    <span className="break-all font-medium text-foreground">{effectiveSlug}</span>. Letters,
                                                    numbers and hyphens only — it has to be unique across Cackle.
                                                </>
                                            ) : (
                                                'Made from your name automatically. Letters, numbers and hyphens only.'
                                            )}
                                        </FormDescription>
                                        <FormMessage />
                                    </FormItem>
                                )}
                            />

                            <div className="space-y-2">
                                <Label htmlFor="org-default-currency">Currency you usually sell in</Label>
                                <Select
                                    value={form.watch('default_currency')}
                                    onValueChange={(value: string) => form.setValue('default_currency', value, { shouldDirty: true })}
                                    disabled={submitting}
                                >
                                    <SelectTrigger id="org-default-currency">
                                        <SelectValue placeholder="Pick a currency" />
                                    </SelectTrigger>
                                    <SelectContent>
                                        {currencyOptions.map((c) => (
                                            <SelectItem key={c.code} value={c.code}>
                                                {c.code} — {c.name}
                                            </SelectItem>
                                        ))}
                                    </SelectContent>
                                </Select>
                                <p className="text-sm text-muted-foreground">
                                    Just the starting point for new events — you can set a different currency per event.
                                </p>
                            </div>

                            <div className="flex flex-col-reverse gap-3 border-t border-border pt-6 sm:flex-row sm:justify-end">
                                {!isFirstOrg && (
                                    <Button type="button" variant="outline" onClick={() => navigate('/admin')} disabled={submitting}>
                                        Cancel
                                    </Button>
                                )}
                                <Button type="submit" disabled={submitting}>
                                    {submitting ? (
                                        <>
                                            <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                                            Creating…
                                        </>
                                    ) : (
                                        <>
                                            {isFirstOrg ? 'Create and continue' : 'Create organisation'}
                                            <ArrowRight className="ml-2 h-4 w-4" />
                                        </>
                                    )}
                                </Button>
                            </div>
                        </form>
                    </Form>
                </CardContent>
            </Card>

            {isFirstOrg && (
                <p className="mt-6 text-center text-sm text-muted-foreground">
                    You&apos;ll be the owner. You can invite your team — and give door staff scanner-only access — once this is done.
                </p>
            )}
        </div>
    );
};

export default CreateOrgPage;
