import { createClient } from '@supabase/supabase-js';

const supabaseUrl = 'REDACTED_SUPABASE_URL';
const supabaseKey = 'REDACTED_SUPABASE_ANON_KEY';
export let supabase = createClient(supabaseUrl, supabaseKey);