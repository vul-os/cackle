import React from 'react';
import { CreditCard, Banknote, Wallet, ArrowRight } from 'lucide-react';
import { Card, CardContent } from '@/components/ui/card';
import PaymentCalculator from './payment-calculator';

const PricingPage = () => {
  const paymentOptions = [
    {
      type: 'Cards',
      description: 'Credit and Debit card payments',
      icon: <CreditCard className="h-12 w-12 text-red-500" />,
      logos: [
        {
          name: 'Visa',
          src: '/images/visa.jpg',
          alt: 'Visa logo'
        },
        {
          name: 'Mastercard',
          src: '/images/master.jpg',
          alt: 'Mastercard logo'
        }
      ],
      provider: {
        name: 'Paystack',
        logo: '/images/paystack.jpg',
      },
      rates: [
        {
          name: 'Local Transactions',
          description: 'South African cards',
          fee: '2.9% + R1.00'
        },
        {
          name: 'International Transactions',
          description: 'Non-South African cards',
          fee: '3.9% + R1.00'
        }
      ]
    },
    {
      type: 'Cash Options',
      description: 'Secure cash payment methods',
      icon: <Banknote className="h-12 w-12 text-red-500" />,
      logos: [
        {
          name: 'Bank Transfer',
          src: '/images/bank.jpg',
          alt: 'Bank transfer icon'
        }
      ],
      provider: {
        name: 'Mukuru Pay',
        logo: '/images/mukuru.jpg',
      },
      rates: [
        {
          name: 'Cash Deposit',
          description: 'Pay at any major retailer',
          fee: '3.9% + R6'
        },
        {
          name: '',
          description: '',
          fee: ''
        }
      ]
    },
    {
      type: 'Instant Payments',
      description: 'Quick digital transfers',
      icon: <Wallet className="h-12 w-12 text-red-500" />,
      logos: [
        {
          name: 'PayShap',
          src: '/images/payshap.jpg',
          alt: 'PayShap logo'
        },
        {
          name: 'Instant EFT',
          src: '/images/eft.jpg',
          alt: 'EFT icon'
        }
      ],
      provider: {
        name: 'PayFast',
        logo: '/images/payfastlogo.jpg',
      },
      rates: [
        {
          name: 'PayShap',
          description: 'Instant mobile payments',
          fee: 'R7.50'
        },
        {
          name: 'Instant EFT',
          description: 'Direct bank transfer',
          fee: '2.0%(min R2)'
        }
      ]
    }
  ];

  return (
    <div className="min-h-screen bg-gradient-to-br from-red-50 via-white to-red-50">
      {/* Promotional Banner */}
      <div className="bg-gradient-to-r from-red-600 to-red-800 text-white">
        <div className="max-w-5xl mx-auto px-4 py-3">
          <div className="flex items-center justify-center gap-4">
            <div className="flex items-center">
              <span className="text-2xl font-bold line-through opacity-75">2%</span>
              <ArrowRight className="mx-2 h-5 w-5" />
              <span className="text-3xl font-bold">0.85%</span>
            </div>
            <div className="h-8 w-px bg-red-400/30" />
            <p className="text-sm font-medium">
              South Africa's Most Competitive Payment Rates
            </p>
          </div>
        </div>
      </div>

      <div className="max-w-5xl mx-auto px-4 py-12">
        {/* Header Section */}
        <div className="text-center mb-12">
          <span className="text-red-600 font-semibold text-sm tracking-wider uppercase mb-4 block">
            Payment Solutions
          </span>
          <h1 className="text-4xl font-bold mb-4 bg-gradient-to-r from-red-600 to-red-800 bg-clip-text text-transparent">
            Payment Methods
          </h1>
          <p className="text-gray-600 max-w-2xl mx-auto">
            Choose from our range of secure payment options tailored for your needs
          </p>
        </div>

        {/* Payment Options Grid */}
        <div className="grid grid-cols-1 md:grid-cols-3 gap-6">
          {paymentOptions.map((option, index) => (
            <Card 
              key={index} 
              className="bg-white/80 backdrop-blur-sm border-0 hover:shadow-xl transition-all duration-300 overflow-hidden"
            >
              <CardContent className="p-6">
                {/* Provider Logo Section */}
                <div className="flex items-center justify-between mb-6">
                  <div className="bg-red-50 p-4 rounded-2xl">
                    {option.icon}
                  </div>
                  <div className="bg-gray-50 p-3 rounded-xl">
                    <img 
                      src={option.provider.logo} 
                      alt={`${option.provider.name} logo`}
                      className="h-8 w-auto object-contain"
                    />
                  </div>
                </div>
                
                <div className="space-y-6">
                  <div>
                    <h3 className="text-xl font-bold text-gray-800 mb-1">
                      {option.type}
                    </h3>
                    <p className="text-sm text-gray-600">
                      {option.description}
                    </p>
                  </div>

                  {/* Payment Method Logos */}
                  <div className="flex items-center gap-4">
                    {option.logos.map((logo, logoIndex) => (
                      <div key={logoIndex} className="bg-white p-2 rounded-lg shadow-sm">
                        <img 
                          src={logo.src} 
                          alt={logo.alt}
                          className="h-6 w-auto object-contain"
                        />
                      </div>
                    ))}
                  </div>
                  
                  {/* Rates Section */}
                  <div className="space-y-3">
                    {option.rates.map((rate, rateIndex) => (
                      rate.name && (
                        <div 
                          key={rateIndex}
                          className="pt-3 border-t border-red-100"
                        >
                          <div className="flex justify-between items-start">
                            <div>
                              <h4 className="font-semibold text-sm text-gray-800 mb-1">
                                {rate.name}
                              </h4>
                              <p className="text-xs text-gray-600">
                                {rate.description}
                              </p>
                            </div>
                            <span className="font-bold text-sm text-red-600">
                              {rate.fee}
                            </span>
                          </div>
                        </div>
                      )
                    ))}
                  </div>

                  {/* Sponsored Section */}
                  <div className="pt-4 border-t border-red-100">
                    <p className="text-xs text-gray-500 text-center">
                      Powered by <span className="font-semibold text-red-600">{option.provider.name}</span>
                    </p>
                  </div>
                </div>
              </CardContent>
            </Card>
          ))}
        </div>

        {/* Calculator Component */}
        <PaymentCalculator />

        {/* Bottom Section */}
        <Card className="mt-12 bg-white/80 backdrop-blur-sm border-0">
          <CardContent className="p-6">
            <div className="grid md:grid-cols-2 gap-8">
              <div>
                <h3 className="text-xl font-bold text-gray-800 mb-4">Secure Payments</h3>
                <p className="text-gray-600">
                  All transactions are processed through our verified partners,
                  ensuring the highest level of security for your payments.
                </p>
              </div>
              <div className="border-l border-red-100 pl-8">
                <h3 className="text-xl font-bold text-gray-800 mb-4">24/7 Support</h3>
                <p className="text-gray-600">
                  Our dedicated support team is available around the clock
                  to assist you with any payment-related queries.
                </p>
              </div>
            </div>
          </CardContent>
        </Card>
      </div>
    </div>
  );
};

export default PricingPage;