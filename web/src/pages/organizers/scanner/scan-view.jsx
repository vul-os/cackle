import React, { useEffect, useRef, useState } from 'react';
import QrScanner from 'qr-scanner';
import { motion, AnimatePresence } from 'framer-motion';
import {
    Wifi,
    WifiOff,
    ShieldCheck,
    Copy,
    Ban,
    RefreshCw,
    Camera,
    ChevronLeft,
    Keyboard,
    UploadCloud,
} from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import useScanEngine from './use-scan-engine';

const RESULT_STYLE = {
    admitted: { bg: 'bg-emerald-500', ring: 'ring-emerald-400', icon: ShieldCheck, label: 'ADMITTED' },
    duplicate: { bg: 'bg-amber-500', ring: 'ring-amber-400', icon: Copy, label: 'ALREADY SCANNED' },
    invalid: { bg: 'bg-rose-600', ring: 'ring-rose-500', icon: Ban, label: 'INVALID' },
    wrong_event: { bg: 'bg-rose-600', ring: 'ring-rose-500', icon: Ban, label: 'WRONG EVENT' },
};

const TALLY_TILES = [
    { key: 'admitted', label: 'Admitted', color: 'text-emerald-400' },
    { key: 'duplicate', label: 'Duplicate', color: 'text-amber-400' },
    { key: 'invalid', label: 'Invalid', color: 'text-rose-400', combineWith: 'wrong_event' },
];

const ScanView = ({ event, keyRing, ticketIndex, gateId, onExit }) => {
    const videoRef = useRef(null);
    const scannerRef = useRef(null);
    const [cameraError, setCameraError] = useState(null);
    const [manualOpen, setManualOpen] = useState(false);
    const [manualValue, setManualValue] = useState('');
    const [showFlash, setShowFlash] = useState(false);

    const { online, tally, pendingCount, lastResult, isSyncing, syncNow, handleDecode } = useScanEngine({
        eventId: event.id,
        keyRing,
        ticketIndex,
        gateId,
    });

    useEffect(() => {
        if (!lastResult) return;
        setShowFlash(true);
        const timer = setTimeout(() => setShowFlash(false), lastResult.result === 'admitted' ? 1100 : 1800);
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

    const handleManualSubmit = (e) => {
        e.preventDefault();
        if (manualValue.trim()) {
            handleDecode(manualValue.trim());
            setManualValue('');
        }
    };

    const flash = lastResult && showFlash ? RESULT_STYLE[lastResult.result] : null;

    return (
        <div className="fixed inset-0 z-50 flex flex-col bg-zinc-950 text-white">
            {/* Status bar */}
            <div className="flex items-center justify-between gap-2 border-b border-white/10 px-4 py-3">
                <Button variant="ghost" size="sm" onClick={onExit} className="text-white/70 hover:bg-white/10 hover:text-white">
                    <ChevronLeft className="mr-1 h-5 w-5" />
                    Exit
                </Button>
                <div className="min-w-0 flex-1 truncate text-center text-sm font-medium text-white/70">{event.title}</div>
                <div
                    className={`flex items-center gap-1.5 rounded-full px-3 py-1 text-xs font-semibold ${
                        online ? 'bg-emerald-500/20 text-emerald-400' : 'bg-amber-500/20 text-amber-400'
                    }`}
                >
                    {online ? <Wifi className="h-3.5 w-3.5" /> : <WifiOff className="h-3.5 w-3.5" />}
                    {online ? 'Online' : 'Offline'}
                </div>
            </div>

            {/* Tally */}
            <div className="grid grid-cols-3 gap-px bg-white/10">
                {TALLY_TILES.map(({ key, label, color, combineWith }) => {
                    const value = tally[key] + (combineWith ? tally[combineWith] || 0 : 0);
                    return (
                        <div key={key} className="bg-zinc-950 px-2 py-4 text-center">
                            <div className={`text-4xl font-black tabular-nums sm:text-5xl ${color}`}>{value}</div>
                            <div className="mt-1 text-xs font-semibold uppercase tracking-wider text-white/50">{label}</div>
                        </div>
                    );
                })}
            </div>

            {/* Camera */}
            <div className="relative flex-1 overflow-hidden bg-black">
                <video ref={videoRef} className="absolute inset-0 h-full w-full object-cover" muted playsInline />

                {cameraError && (
                    <div className="absolute inset-0 flex flex-col items-center justify-center gap-3 bg-zinc-950 p-6 text-center">
                        <Camera className="h-10 w-10 text-white/40" />
                        <p className="font-medium">Camera unavailable</p>
                        <p className="max-w-xs text-sm text-white/60">{cameraError}</p>
                        <p className="text-sm text-white/60">Use manual entry below instead.</p>
                    </div>
                )}

                <AnimatePresence>
                    {flash && (
                        <motion.div
                            key={lastResult.id}
                            initial={{ opacity: 0 }}
                            animate={{ opacity: 1 }}
                            exit={{ opacity: 0 }}
                            transition={{ duration: 0.15 }}
                            className={`absolute inset-0 flex flex-col items-center justify-center gap-3 ${flash.bg}/95 px-6 text-center backdrop-blur-sm`}
                        >
                            <motion.div initial={{ scale: 0.6 }} animate={{ scale: 1 }} transition={{ type: 'spring', stiffness: 260, damping: 18 }}>
                                <flash.icon className="h-24 w-24 text-white drop-shadow" strokeWidth={2.5} />
                            </motion.div>
                            <p className="text-3xl font-black tracking-tight text-white sm:text-4xl">{flash.label}</p>
                            {lastResult.holder_name && <p className="text-lg font-medium text-white/90">{lastResult.holder_name}</p>}
                            {lastResult.note && <p className="max-w-sm text-sm text-white/80">{lastResult.note}</p>}
                        </motion.div>
                    )}
                </AnimatePresence>
            </div>

            {/* Footer controls */}
            <div className="space-y-3 border-t border-white/10 p-4">
                {manualOpen && (
                    <form onSubmit={handleManualSubmit} className="flex gap-2">
                        <Input
                            autoFocus
                            value={manualValue}
                            onChange={(e) => setManualValue(e.target.value)}
                            placeholder="Paste capability token..."
                            className="border-white/20 bg-white/10 text-white placeholder:text-white/40"
                        />
                        <Button type="submit">Check</Button>
                    </form>
                )}
                <div className="flex items-center justify-between gap-2 text-sm text-white/60">
                    <button onClick={() => setManualOpen((v) => !v)} className="flex items-center gap-1.5 hover:text-white">
                        <Keyboard className="h-4 w-4" />
                        {manualOpen ? 'Hide manual entry' : 'Enter code manually'}
                    </button>
                    <button
                        onClick={syncNow}
                        disabled={!online || isSyncing || pendingCount === 0}
                        className="flex items-center gap-1.5 disabled:opacity-40"
                    >
                        {isSyncing ? <RefreshCw className="h-4 w-4 animate-spin" /> : <UploadCloud className="h-4 w-4" />}
                        {pendingCount > 0 ? `${pendingCount} pending sync` : 'Synced'}
                    </button>
                </div>
            </div>
        </div>
    );
};

export default ScanView;
