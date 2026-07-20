import { useEffect, useState } from 'react';
import { categories as categoriesApi } from '@/lib/api';

/**
 * `GET /api/categories` -> [{slug,label,count}], with loading/error state.
 * Category tabs are a filter convenience, not critical path — callers
 * should hide the tab row on error rather than blocking the page.
 */
export function useCategories() {
    const [state, setState] = useState({ categories: [], loading: true, error: false });

    useEffect(() => {
        let cancelled = false;
        categoriesApi
            .list()
            .then((data) => {
                if (cancelled) return;
                // docs/API.md specifies a bare array; tolerate a {categories: [...]}
                // envelope too so a shape change on the backend degrades to "no
                // categories" rather than silently hiding real data forever.
                const list = Array.isArray(data) ? data : Array.isArray(data?.categories) ? data.categories : [];
                setState({ categories: list, loading: false, error: false });
            })
            .catch(() => {
                if (cancelled) return;
                setState({ categories: [], loading: false, error: true });
            });
        return () => {
            cancelled = true;
        };
    }, []);

    return state;
}
