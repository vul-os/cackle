import React, { useEffect, useState } from 'react';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { categories as categoriesApi } from '@/lib/api';
import { mergeCategories } from '../categories';

/**
 * Category picker for the event editor + create wizard. Backed by
 * GET /api/categories where available, always falling back to a sensible
 * static list so a brand-new org (with no events to derive categories
 * from) still has something to choose from — a failed fetch here degrades
 * to the fallback list silently rather than blocking the form.
 */
const CategorySelect = ({ value, onChange, disabled, id }) => {
    const [options, setOptions] = useState(() => mergeCategories([]));

    useEffect(() => {
        let cancelled = false;
        categoriesApi
            .list()
            .then((data) => {
                if (cancelled) return;
                const list = Array.isArray(data) ? data : (data?.categories ?? []);
                setOptions(mergeCategories(list));
            })
            .catch(() => {
                // keep the fallback list — categories are a nice-to-have filter
            });
        return () => {
            cancelled = true;
        };
    }, []);

    return (
        <Select value={value || undefined} onValueChange={onChange} disabled={disabled}>
            <SelectTrigger id={id} className="w-full sm:w-64">
                <SelectValue placeholder="Choose a category" />
            </SelectTrigger>
            <SelectContent>
                {options.map((c) => (
                    <SelectItem key={c.slug} value={c.slug}>
                        {c.label}
                    </SelectItem>
                ))}
            </SelectContent>
        </Select>
    );
};

export default CategorySelect;
