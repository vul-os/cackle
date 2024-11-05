import React from 'react';
import { Card, CardContent } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Loader2, CheckCircle2, XCircle, ExternalLink } from 'lucide-react';
import Header from '@/pages/visitor/header';

const THEMES = {
  processing: {
    icon: Loader2,
    iconClass: 'text-blue-500 animate-spin',
    title: 'Verifying Payment',
    description: 'Please wait while we confirm your payment...',
    buttonText: 'Please wait...',
    buttonDisabled: true,
    showExternalLink: false,
  },
  success: {
    icon: CheckCircle2,
    iconClass: 'text-green-500',
    title: 'Payment Successful!',
    description: 'Thank you for your purchase. Your order has been confirmed.',
    buttonText: 'View My Orders',
    buttonDisabled: false,
    showExternalLink: false,
  },
  failed: {
    icon: XCircle,
    iconClass: 'text-red-500',
    title: 'Payment Failed',
    description: 'We were unable to process your payment. Please try again.',
    buttonText: 'Back to Orders',
    buttonDisabled: false,
    showExternalLink: false,
  },
  redirecting: {
    icon: Loader2,
    iconClass: 'text-blue-500 animate-spin',
    title: 'Redirecting to Payment Gateway',
    description: 'Please wait while we redirect you to our secure payment page...',
    buttonText: 'Redirecting...',
    buttonDisabled: true,
    showExternalLink: true,
  }
};

export default function PaymentStatusPage({ 
  theme = 'processing',
  customConfig = {},
  onButtonClick = () => {},
}) {
  // Merge default theme with custom config
  const config = {
    ...THEMES[theme],
    ...customConfig
  };

  const Icon = config.icon;

  return (
    <>
      <Header />
      <div className="min-h-screen bg-gray-50 dark:bg-slate-900">
        <div className="container max-w-lg mx-auto px-4 py-32">
          <Card className="relative overflow-hidden">
            <CardContent className="pt-12 pb-8">
              <div className="flex flex-col items-center text-center space-y-8">
                <Icon className={`h-24 w-24 ${config.iconClass}`} />
                
                <div className="space-y-2">
                  <h1 className="text-2xl font-semibold tracking-tight">
                    {config.title}
                  </h1>
                  <p className="text-base text-gray-500 dark:text-gray-400">
                    {config.description}
                  </p>
                </div>

                {config.showExternalLink && (
                  <div className="flex items-center space-x-2 text-sm text-gray-500">
                    <ExternalLink className="h-4 w-4" />
                    <span>You will be redirected automatically</span>
                  </div>
                )}

                <Button 
                  className="w-full max-w-xs"
                  disabled={config.buttonDisabled}
                  onClick={onButtonClick}
                >
                  {config.buttonDisabled && theme === 'redirecting' && (
                    <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                  )}
                  {config.buttonText}
                </Button>
              </div>
            </CardContent>
          </Card>
        </div>
      </div>
    </>
  );
}
