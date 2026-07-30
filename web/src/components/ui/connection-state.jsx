import React from 'react';
import { Wifi, WifiOff, CloudUpload, Check, Loader2 } from 'lucide-react';
import { cn } from '@/lib/utils';

// How Cackle talks about the network.
//
// This component exists to enforce one product decision in code: **a gate with
// no network is not in an error state.** It is the product working as designed.
// A ticket is a signed capability; the gate verifies it against a key ring it
// already has; the server's opinion is not part of the decision. Rendering that
// situation in warning amber with a broken-cloud icon — which is what almost
// every offline-capable app does — teaches the operator to distrust exactly the
// property they are relying on, and to go looking for a fix at the worst
// possible moment.
//
// So: offline is NEUTRAL and it says what it is doing. Nothing here uses the
// warning or destructive tokens, and the copy never apologises.
//
// The genuinely notable state is the inverse — scans waiting to sync — and even
// that is information, not an error. Those scans are safe on the device and
// have already done their job at the door; syncing is how they reach the
// cross-gate reconciliation report later.

/**
 * ConnectionState renders the gate's relationship with the server.
 *
 * @param {object} props
 * @param {boolean} props.online
 * @param {number} [props.pendingCount] scans recorded locally, not yet synced
 * @param {boolean} [props.syncing]
 * @param {'chip'|'row'} [props.variant] chip for a dense bar, row for a panel
 * @param {'light'|'gate'} [props.surface] which palette to render against
 */
export function ConnectionState({ online, pendingCount = 0, syncing = false, variant = 'chip', surface = 'light', className }) {
    const label = online ? 'Online' : 'Offline — admitting normally';
    const Icon = online ? Wifi : WifiOff;

    // On the gate's fixed dark surface the neutral treatment is a white wash;
    // on ordinary themed pages it is the muted token. Neither is amber.
    const neutral =
        surface === 'gate'
            ? 'bg-white/10 text-white'
            : 'bg-muted text-muted-foreground';
    const positive = surface === 'gate' ? 'bg-white/10 text-white' : 'bg-success/10 text-success';

    if (variant === 'row') {
        return (
            <div className={cn('flex flex-wrap items-center gap-x-3 gap-y-1 text-sm', className)}>
                <span className="inline-flex items-center gap-2 font-medium">
                    <Icon className="h-4 w-4 shrink-0" aria-hidden="true" />
                    {label}
                </span>
                {!online && (
                    <span className={surface === 'gate' ? 'text-white/70' : 'text-muted-foreground'}>
                        Tickets are verified on this device. No connection is needed to admit anyone.
                    </span>
                )}
                <SyncState pendingCount={pendingCount} syncing={syncing} surface={surface} />
            </div>
        );
    }

    return (
        <span
            className={cn(
                'inline-flex items-center gap-1.5 rounded-full px-3 py-1.5 text-xs font-semibold',
                online ? positive : neutral,
                className,
            )}
        >
            <Icon className="h-3.5 w-3.5 shrink-0" aria-hidden="true" />
            {label}
        </span>
    );
}

/**
 * SyncState describes the local queue. Phrased as a fact about where the
 * scans are, not as a problem to solve — they are already recorded, and the
 * door has already been dealt with.
 */
export function SyncState({ pendingCount = 0, syncing = false, surface = 'light', className }) {
    const muted = surface === 'gate' ? 'text-white/70' : 'text-muted-foreground';

    if (syncing) {
        return (
            <span className={cn('inline-flex items-center gap-1.5 text-sm', muted, className)}>
                <Loader2 className="h-3.5 w-3.5 animate-spin" aria-hidden="true" />
                Sending scans…
            </span>
        );
    }
    if (pendingCount > 0) {
        return (
            <span className={cn('inline-flex items-center gap-1.5 text-sm', muted, className)}>
                <CloudUpload className="h-3.5 w-3.5" aria-hidden="true" />
                <span className="tnum">{pendingCount}</span>
                {pendingCount === 1 ? ' scan held on this device' : ' scans held on this device'}
            </span>
        );
    }
    return (
        <span className={cn('inline-flex items-center gap-1.5 text-sm', muted, className)}>
            <Check className="h-3.5 w-3.5" aria-hidden="true" />
            All scans sent
        </span>
    );
}

export default ConnectionState;
