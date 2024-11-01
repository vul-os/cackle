import React, { useState, useRef, useEffect } from 'react';
import { Camera } from 'lucide-react';
import { Alert, AlertDescription } from '@/components/ui/alert';
import { Card } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import QrScanner from 'qr-scanner';

const QRScannerPage = () => {
  const [hasPermission, setHasPermission] = useState(false);
  const [scanning, setScanning] = useState(false);
  const [result, setResult] = useState('');
  const [error, setError] = useState('');
  const videoRef = useRef(null);
  const scannerRef = useRef(null);
  
  const startScanner = async () => {
    try {
      if (!videoRef.current) return;
      
      // Create new scanner instance
      scannerRef.current = new QrScanner(
        videoRef.current,
        result => {
          setResult(result.data);
          // Optional: Uncomment to stop scanning after finding a code
          // stopScanner();
        },
        {
          preferredCamera: 'environment',
          highlightScanRegion: true,
          highlightCodeOutline: true,
          maxScansPerSecond: 5,
        }
      );
      
      await scannerRef.current.start();
      setScanning(true);
      setHasPermission(true);
      setError('');
      
    } catch (err) {
      console.error('Error starting scanner:', err);
      setError(`Scanner error: ${err.message}`);
      setHasPermission(false);
    }
  };

  const stopScanner = () => {
    if (scannerRef.current) {
      scannerRef.current.stop();
      scannerRef.current.destroy();
      scannerRef.current = null;
    }
    setScanning(false);
  };

  useEffect(() => {
    return () => {
      stopScanner();
    };
  }, []);

  return (
    <div className="min-h-screen bg-slate-50 p-4">
      <Card className="max-w-md mx-auto p-6 space-y-4">
        <div className="text-center space-y-2">
          <Camera className="w-12 h-12 mx-auto text-slate-600" />
          <h1 className="text-2xl font-bold">QR Code Scanner</h1>
        </div>

        {error && (
          <Alert variant="destructive">
            <AlertDescription>{error}</AlertDescription>
          </Alert>
        )}

        <div className="space-y-4">
          <div className="relative aspect-square bg-black rounded-lg overflow-hidden">
            <video 
              ref={videoRef}
              className="absolute inset-0 w-full h-full object-cover"
            />
            
            {scanning && (
              <div className="absolute bottom-4 left-0 right-0 text-center">
                <p className="text-white bg-black/50 py-2 mx-4 rounded-full text-sm">
                  Looking for QR code...
                </p>
              </div>
            )}
          </div>
          
          {result && (
            <Alert className="animate-in fade-in slide-in-from-bottom-4">
              <AlertDescription className="break-all">
                Found QR Code: {result}
              </AlertDescription>
            </Alert>
          )}
          
          <Button
            onClick={() => {
              if (scanning) {
                stopScanner();
              } else {
                startScanner();
              }
            }}
            className="w-full"
          >
            {scanning ? 'Stop Scanner' : 'Start Scanner'}
          </Button>

          {/* Debug info */}
          <div className="text-sm text-slate-500 space-y-1">
            <p>Scanner Status: {scanning ? 'Active' : 'Inactive'}</p>
            <p>Permission Status: {hasPermission ? 'Granted' : 'Not Granted'}</p>
          </div>
        </div>
      </Card>
    </div>
  );
};

export default QRScannerPage;