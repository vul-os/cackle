import React, { useCallback, useEffect, useRef, useState } from 'react';
import QrScanner from 'qr-scanner';
import { motion, AnimatePresence, useReducedMotion } from 'framer-motion';
import { Camera, ChevronLeft, Keyboard, ShieldCheck, X } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { ConnectionState, SyncState } from '@/components/ui/connection-state';
import useScanEngine from './use-scan-engine';
import { verdictFor, verdictHoldMs, describeDuplicate } from './verdict';

// The gate.
//
// Every decision on this screen is downstream of one sentence: it is used on a
// phone, held at arm's length, outdoors, in bad light, at speed, by somebody
// with a queue in front of them and possibly no network. That is not a
// degraded mode to be handled — it is the design centre.
//
// What that produced:
//
//  - FIXED DARK SURFACE, both themes. A light UI behind a camera preview is
//    glare. The verdict colours are pinned too (see index.css), so the answer
//    never changes shade because of a theme toggle nobody touched.
//
//  - THE VERDICT IS THE WHOLE SCREEN. Not a toast, not a row — a full-bleed
//    flood with a 96px glyph and a verb. It is readable without focusing on it.
//
//  - NO ROUND TRIP, EVER. `useScanEngine` verifies the capability locally
//    against the cached key ring. There is no request between the scan and the
//    answer, and there is nothing on this screen that waits for one.
//
//  - OFFLINE IS NOT AN ERROR STATE. See components/ui/connection-state.tsx.
//
//  - COLOUR IS NEVER THE ONLY SIGNAL — icon, word and haptic pattern all
//    differ per verdict. See ./verdict.ts.
//
//  - BIG TARGETS. Everything tappable here is at least 56px, comfortably over
//    the 44px WCAG 2.5.5 floor, because the person using it may be wearing
//    gloves and is definitely not looking carefully.

/** Minimum tap target on this screen. Gloves, cold, speed. */
const TAP = 'min-h-[56px] min-w-[56px]';

// This screen never follows the OS theme (see the file banner above), so its
// ink can't come from --foreground/--muted-foreground either — those flip
// with `.dark`. It uses the fixed `bg-gate` / `text-gate-ink` /
// `text-gate-ink-muted` / `border-gate-line` / `bg-gate-accent` utilities
// (tailwind.config.js) rather than a raw hardcoded colour class — see the
// "gate" colour group there for why. None of this is the verdict palette:
// `--verdict-*` stays reserved for the full-bleed flood alone.
const GATE_GHOST_BUTTON = 'text-gate-ink hover:bg-gate-accent hover:text-gate-ink';

/**
 * TallyTile is one of the three running counts.
 *
 * Deliberately labelled as THIS gate's numbers. Scan history is per-device
 * until it reconciles, and an organiser who reads a single gate's tally as the
 * event total will be wrong by however many people came through the other
 * doors. Saying so costs one line.
 *
 * The three counts are told apart by their LABEL, not by colour: colour here
 * would have to come from either the verdict palette (reserved for the
 * full-bleed flood alone — see index.css's "Fills only… never re-used for
 * ordinary chrome") or from --success/--warning/--destructive, which flip
 * with the OS theme and are not guaranteed to hold contrast against this
 * screen's fixed near-black ground in every theme. A single fixed ink avoids
 * both problems and keeps the promise that this surface never changes shade
 * for a reason nobody touched.
 */
function TallyTile({ value, label }) {
    return (
        <div className="bg-transparent px-2 py-3 text-center">
            <div className="tnum text-3xl font-black leading-none text-gate-ink sm:text-4xl">{value}</div>
            <div className="mt-1 text-[12px] font-semibold uppercase tracking-wider text-gate-ink-muted">{label}</div>
        </div>
    );
}

/**
 * VerdictFlood is the answer, full screen.
 *
 * `aria-live="assertive"` because this is the one thing on the page worth
 * interrupting a screen reader for, and because a gate may well be operated by
 * somebody using VoiceOver with the screen off in bright sun.
 */
function VerdictFlood({ result, onDismiss, reduceMotion }) {
    const verdict = verdictFor(result.result);
    const Icon = verdict.icon;
    const duplicate = result.result === 'duplicate' ? describeDuplicate(result.note) : null;

    return (
        <motion.div
            key={result.id}
            initial={reduceMotion ? false : { opacity: 0 }}
            animate={{ opacity: 1 }}
            exit={reduceMotion ? { opacity: 1 } : { opacity: 0 }}
            transition={{ duration: reduceMotion ? 0 : 0.12 }}
            // Tapping anywhere clears it. On a moving queue the operator's
            // hand is already on the screen; making them find a button to get
            // back to the camera costs a second per person.
            onClick={onDismiss}
            role="alertdialog"
            aria-live="assertive"
            aria-label={verdict.label}
            className={`absolute inset-0 z-20 flex flex-col items-center justify-center gap-4 px-6 text-center text-verdict-ink ${verdict.surface}`}
        >
            <motion.div
                initial={reduceMotion ? false : { scale: 0.7 }}
                animate={{ scale: 1 }}
                transition={reduceMotion ? { duration: 0 } : { type: 'spring', stiffness: 260, damping: 18 }}
            >
                <Icon className="h-24 w-24" strokeWidth={2.5} aria-hidden="true" />
            </motion.div>

            <p className="font-display text-3xl font-black leading-tight tracking-tight sm:text-5xl">{verdict.label}</p>

            {result.holder_name && <p className="text-xl font-semibold sm:text-2xl">{result.holder_name}</p>}

            {duplicate ? (
                <p className="max-w-sm text-base font-medium">{duplicate.detail}</p>
            ) : (
                result.note && <p className="max-w-sm text-base font-medium">{result.note}</p>
            )}

            <p className="mt-2 text-sm font-semibold uppercase tracking-wide opacity-80">Tap to scan the next one</p>
        </motion.div>
    );
}

const ScanView = ({ event, keyRing, ticketIndex, ticketIndexPresent, admittedIndex, gateId, onExit }) => {
    const videoRef = useRef(null);
    const scannerRef = useRef(null);
    const [cameraError, setCameraError] = useState(null);
    const [manualOpen, setManualOpen] = useState(false);
    const [manualValue, setManualValue] = useState('');
    const [flooding, setFlooding] = useState(false);
    const reduceMotion = useReducedMotion();

    const { online, tally, pendingCount, lastResult, isSyncing, syncNow, handleDecode } = useScanEngine({
        eventId: event.id,
        keyRing,
        ticketIndex,
        ticketIndexPresent,
        admittedIndex,
        gateId,
    });

    // Show the verdict, buzz, and clear it after its own hold time. Admissions
    // clear fast (the queue is moving); refusals linger long enough to be read.
    useEffect(() => {
        if (!lastResult) return;
        setFlooding(true);

        const verdict = verdictFor(lastResult.result);
        // Haptics are progressive enhancement: absent on iOS Safari, present
        // on most Android gate phones, and never the only channel.
        if (typeof navigator !== 'undefined' && typeof navigator.vibrate === 'function') {
            try {
                navigator.vibrate(verdict.vibrate);
            } catch {
                // A browser that exposes vibrate but refuses it is not a
                // reason to stop admitting people.
            }
        }

        const timer = setTimeout(() => setFlooding(false), verdictHoldMs(lastResult.result));
        return () => clearTimeout(timer);
    }, [lastResult]);

    useEffect(() => {
        let cancelled = false;
        (async () => {
            if (!videoRef.current) return;
            try {
                scannerRef.current = new QrScanner(videoRef.current, (result) => handleDecode(result.data), {
                    preferredCamera: 'environment',
                    highlightScanRegion: true,
                    highlightCodeOutline: true,
                    maxScansPerSecond: 6,
                });
                await scannerRef.current.start();
                if (cancelled) scannerRef.current.stop();
            } catch (err) {
                if (!cancelled) setCameraError(err.message || 'Could not access the camera.');
            }
        })();

        return () => {
            cancelled = true;
            if (scannerRef.current) {
                scannerRef.current.stop();
                scannerRef.current.destroy();
                scannerRef.current = null;
            }
        };
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, []);

    // A camera that will not start is a real failure — it is the input device.
    // Manual entry opens automatically so the gate keeps working instead of
    // presenting a dead end.
    useEffect(() => {
        if (cameraError) setManualOpen(true);
    }, [cameraError]);

    const handleManualSubmit = useCallback(
        (e) => {
            e.preventDefault();
            const value = manualValue.trim();
            if (!value) return;
            handleDecode(value);
            setManualValue('');
        },
        [manualValue, handleDecode],
    );

    return (
        <div className="gate-surface fixed inset-0 z-50 flex flex-col">
            {/* ── Bar ─────────────────────────────────────────────────────── */}
            <header className="flex items-center justify-between gap-2 border-b border-gate-line px-2 py-2">
                <Button variant="ghost" onClick={onExit} className={`${TAP} px-3 ${GATE_GHOST_BUTTON}`}>
                    <ChevronLeft className="mr-1 h-6 w-6" aria-hidden="true" />
                    Exit
                </Button>

                <div className="min-w-0 flex-1 text-center">
                    <p className="truncate text-sm font-semibold text-gate-ink">{event.title}</p>
                    <p className="truncate text-xs text-gate-ink-muted">{gateId}</p>
                </div>

                <ConnectionState online={online} surface="gate" className="shrink-0" />
            </header>

            {/* ── This gate's running count ───────────────────────────────── */}
            <section aria-label="Counts for this gate" className="border-b border-gate-line">
                <div className="grid grid-cols-3 divide-x divide-gate-line">
                    <TallyTile value={tally.admitted} label="Admitted" />
                    <TallyTile value={tally.duplicate} label="Already in" />
                    <TallyTile value={tally.invalid + (tally.wrong_event || 0)} label="Refused" />
                </div>
                <p className="px-3 pb-2 text-center text-[12px] text-gate-ink-muted">
                    This gate only. Other gates keep their own count until everything syncs.
                </p>
            </section>

            {/* ── Camera + verdict ────────────────────────────────────────── */}
            <div className="relative flex-1 overflow-hidden bg-gate">
                <video ref={videoRef} className="absolute inset-0 h-full w-full object-cover" muted playsInline />

                {!cameraError && !flooding && (
                    <div className="pointer-events-none absolute inset-0 flex items-center justify-center">
                        <div className="h-56 w-56 rounded-3xl border-4 border-gate-ink shadow-[0_0_0_100vmax_hsl(var(--gate-bg)/0.45)]" />
                    </div>
                )}

                {cameraError && (
                    <div className="absolute inset-0 flex flex-col items-center justify-center gap-3 bg-gate p-6 text-center">
                        <Camera className="h-12 w-12 text-gate-ink-muted" aria-hidden="true" />
                        <p className="text-lg font-semibold text-gate-ink">Camera unavailable</p>
                        <p className="max-w-xs text-sm text-gate-ink-muted">{cameraError}</p>
                        <p className="max-w-xs text-sm text-gate-ink-muted">
                            Type or paste the ticket code below — it is checked exactly the same way.
                        </p>
                    </div>
                )}

                <AnimatePresence>
                    {flooding && lastResult && (
                        <VerdictFlood
                            result={lastResult}
                            reduceMotion={reduceMotion}
                            onDismiss={() => setFlooding(false)}
                        />
                    )}
                </AnimatePresence>
            </div>

            {/* ── Controls ────────────────────────────────────────────────── */}
            <footer className="space-y-3 border-t border-gate-line p-3">
                {manualOpen && (
                    <form onSubmit={handleManualSubmit} className="flex gap-2">
                        <label htmlFor="manual-token" className="sr-only">
                            Ticket code
                        </label>
                        <Input
                            id="manual-token"
                            autoFocus
                            value={manualValue}
                            onChange={(e) => setManualValue(e.target.value)}
                            placeholder="cackle.…"
                            autoCapitalize="none"
                            autoCorrect="off"
                            spellCheck={false}
                            className={`${TAP} border-gate-line bg-gate-accent text-base text-gate-ink placeholder:text-gate-ink-muted`}
                        />
                        <Button type="submit" className={TAP}>
                            Check
                        </Button>
                    </form>
                )}

                <div className="flex items-center justify-between gap-3">
                    <Button
                        type="button"
                        variant="ghost"
                        onClick={() => setManualOpen((v) => !v)}
                        className={`${TAP} px-3 ${GATE_GHOST_BUTTON}`}
                    >
                        {manualOpen ? (
                            <X className="mr-2 h-5 w-5" aria-hidden="true" />
                        ) : (
                            <Keyboard className="mr-2 h-5 w-5" aria-hidden="true" />
                        )}
                        {manualOpen ? 'Close' : 'Type a code'}
                    </Button>

                    {/* Sync runs by itself — on reconnect and after every scan
                        — so this is not a step the operator has to remember.
                        It becomes a button only when there is genuinely
                        something to retry and a connection to retry it on;
                        otherwise it is a plain status line, because a
                        permanently tappable "sync now" invites the belief that
                        admitting depends on it. It does not. */}
                    {online && pendingCount > 0 && !isSyncing ? (
                        <Button
                            type="button"
                            variant="ghost"
                            onClick={syncNow}
                            className={`${TAP} px-3 ${GATE_GHOST_BUTTON}`}
                        >
                            <SyncState pendingCount={pendingCount} syncing={false} surface="gate" />
                            <span className="ml-2 underline underline-offset-4">Send now</span>
                        </Button>
                    ) : (
                        <SyncState pendingCount={pendingCount} syncing={isSyncing} surface="gate" />
                    )}
                </div>

                <p className="flex items-center justify-center gap-1.5 text-center text-[12px] leading-snug text-gate-ink-muted">
                    <ShieldCheck className="h-3.5 w-3.5 shrink-0" aria-hidden="true" />
                    Each ticket is checked against this event&apos;s signing keys, on this phone.
                </p>
            </footer>
        </div>
    );
};

export default ScanView;
