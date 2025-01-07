import React from 'react';
import { CreditCard, ArrowRight } from 'lucide-react';
import { Card, CardContent } from '@/components/ui/card';
import Footer from '@/pages/visitor/landing/footer.jsx';
import Header from '@/pages/visitor/header.jsx';

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
    }
  ];

  return (
    <div className="min-h-screen flex flex-col">
      <div className="fixed w-full z-50">
        <Header />
      </div>

      <div className="flex flex-col pt-16">
        {/* Promotional Banner */}
        <div className="w-full bg-gradient-to-r from-red-600 to-red-800">
          <div className="max-w-5xl mx-auto px-4 py-4">
            <div className="flex items-center justify-center gap-6">
              <div className="flex items-center">
                <span className="text-2xl font-bold text-white/80 line-through">2%</span>
                <ArrowRight className="mx-3 h-6 w-6 text-white" />
                <span className="text-3xl font-bold text-white">0.85%</span>
              </div>
              <div className="h-8 w-px bg-red-400/30" />
              <p className="text-lg font-medium text-white">
                South Africa's Most Competitive Payment Rates
              </p>
            </div>
          </div>
        </div>

        {/* Main Content */}
        <main className="flex-grow bg-gray-50">
          <div className="max-w-5xl mx-auto px-4 py-16">
            {/* Page Header */}
            <div className="text-center mb-16">
              <span className="text-red-600 font-semibold text-sm tracking-wider uppercase mb-4 block">
                Payment Solutions
              </span>
              <h1 className="text-4xl md:text-5xl font-bold mb-6 bg-gradient-to-r from-red-600 to-red-800 bg-clip-text text-transparent">
                Payment Methods
              </h1>
              <p className="text-gray-600 text-lg max-w-2xl mx-auto">
                Choose from our range of secure payment options tailored for your needs
              </p>
            </div>

            {/* Payment Options */}
            <div className="mb-16">
              {paymentOptions.map((option, index) => (
                <Card 
                  key={index} 
                  className="bg-white shadow-lg border-0"
                >
                  <CardContent className="p-8">
                    {/* Provider Logo Section */}
                    <div className="flex items-center justify-between mb-8">
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
                    
                    <div className="space-y-8">
                      <div>
                        <h3 className="text-2xl font-bold text-gray-800 mb-2">
                          {option.type}
                        </h3>
                        <p className="text-gray-600">
                          {option.description}
                        </p>
                      </div>

                      {/* Payment Method Logos */}
                      <div className="flex items-center gap-6">
                        {option.logos.map((logo, logoIndex) => (
                          <div key={logoIndex} className="bg-white p-3 rounded-lg shadow">
                            <img 
                              src={logo.src} 
                              alt={logo.alt}
                              className="h-8 w-auto object-contain"
                            />
                          </div>
                        ))}
                      </div>
                      
                      {/* Rates Section */}
                      <div className="space-y-4">
                        {option.rates.map((rate, rateIndex) => (
                          rate.name && (
                            <div 
                              key={rateIndex}
                              className="pt-4 border-t border-gray-100 first:border-t-0 first:pt-0"
                            >
                              <div className="flex justify-between items-start">
                                <div>
                                  <h4 className="font-semibold text-gray-800 mb-1">
                                    {rate.name}
                                  </h4>
                                  <p className="text-sm text-gray-600">
                                    {rate.description}
                                  </p>
                                </div>
                                <span className="font-bold text-lg text-red-600">
                                  {rate.fee}
                                </span>
                              </div>
                            </div>
                          )
                        ))}
                      </div>

                      {/* Sponsored Section */}
                      <div className="pt-6 border-t border-gray-100">
                        <p className="text-sm text-gray-500 text-center">
                          Powered by <span className="font-semibold text-red-600">{option.provider.name}</span>
                        </p>
                      </div>
                    </div>
                  </CardContent>
                </Card>
              ))}
            </div>

            {/* Features Section */}
            <Card className="bg-white shadow-lg border-0">
              <CardContent className="p-8">
                <div className="grid md:grid-cols-2 gap-8 md:gap-12">
                  <div>
                    <h3 className="text-2xl font-bold text-gray-800 mb-4">Secure Payments</h3>
                    <p className="text-gray-600 leading-relaxed">
                      All transactions are processed through our verified partners,
                      ensuring the highest level of security for your payments.
                      We use industry-standard encryption to protect your data.
                    </p>
                  </div>
                  <div className="md:border-l border-gray-200 md:pl-12">
                    <h3 className="text-2xl font-bold text-gray-800 mb-4">24/7 Support</h3>
                    <p className="text-gray-600 leading-relaxed">
                      Our dedicated support team is available around the clock
                      to assist you with any payment-related queries. Get help
                      whenever you need it.
                    </p>
                  </div>
                </div>
              </CardContent>
            </Card>
          </div>
        </main>
        
        {/* Footer */}
        <Footer />
      </div>
    </div>
  );
};

export default PricingPage;