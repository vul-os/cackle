/// <reference types="vite/client" />

interface ImportMetaEnv {
    readonly VITE_API_URL?: string;
    // Unset by default — see src/pages/visitor/events/location.tsx for why
    // no tile host is contacted unless an operator names their own.
    readonly VITE_CACKLE_MAP_TILE_URL?: string;
    readonly VITE_CACKLE_MAP_TILE_ATTRIBUTION?: string;
}

interface ImportMeta {
    readonly env: ImportMetaEnv;
}
