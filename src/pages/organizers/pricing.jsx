import React, { useState } from 'react';
import { Check, ChevronDown, ChevronUp, Info } from 'lucide-react';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';

const PricingPage = () => {
  const [expandedMethod, setExpandedMethod] = useState(null);

  const paymentMethods = [
    {
      type: 'PayFast',
      description: 'Multiple payment options in one',
      logo: '/api/placeholder/120/40',
      methods: [
        {
          name: 'Credit and Cheque Card',
          description: "The world's most popular online payment method.",
          fee: '3.2% plus R2.00'
        },
        {
          name: 'Instant EFT',
          description: 'An electronic funds transfer that gets verified instantly.',
          fee: '2.0% (min R2.00)'
        },
        {
          name: 'Debit Card',
          description: 'A popular payment option for many South African shoppers.',
          fee: '3.5% plus R2.00'
        },
        {
          name: 'Apple Pay',
          description: "A popular mobile wallet that's linked to an online shopper's Apple device.",
          fee: '3.2% (min R2.00)'
        },
        {
          name: 'Samsung Pay',
          description: 'A mobile payment solution for online shoppers with Samsung Galaxy smartphones.',
          fee: '3.2% (min R2.00)'
        },
        {
          name: 'Masterpass',
          description: 'A digital wallet that streamlines the checkout process.',
          fee: '3.5% plus R2.00'
        }
      ]
    },
    {
      type: 'Shape Pay',
      description: 'Buy Now, Pay Later',
      logo: '/api/placeholder/120/40',
      methods: [
        {
          name: 'MoreTyme',
          description: 'A buy now, pay later option with TymeBank.',
          fee: '5.5% plus R2.00'
        }
      ]
    }
  ];

  return (
    <div className="w-full max-w-5xl mx-auto px-4 py-16">
      <div className="text-center mb-12">
        <h1 className="text-4xl font-bold mb-4">Simple, Transparent Pricing</h1>
        <p className="text-lg text-gray-600 mb-2">Our fee is always 0.85%</p>
        <p className="text-gray-500">Plus payment processing fees</p>
      </div>

      <div className="space-y-6 mb-12">
        {paymentMethods.map((provider, index) => (
          <Card key={index} className="bg-white shadow-lg overflow-hidden">
            <div 
              className="cursor-pointer"
              onClick={() => setExpandedMethod(expandedMethod === index ? null : index)}
            >
              <CardHeader className="flex flex-row items-center justify-between pb-4">
                <div className="flex items-center space-x-4">
                  <img 
                    src={provider.logo} 
                    alt={`${provider.type} logo`}
                    className="h-10"
                  />
                  <div>
                    <CardTitle className="text-xl font-bold">{provider.type}</CardTitle>
                    <p className="text-sm text-gray-500">{provider.description}</p>
                  </div>
                </div>
                <div className="flex items-center space-x-2">
                  <span className="font-semibold text-blue-600">Our fee: 0.85%</span>
                  {expandedMethod === index ? <ChevronUp /> : <ChevronDown />}
                </div>
              </CardHeader>
            </div>
            
            {expandedMethod === index && (
              <CardContent className="border-t">
                <div className="space-y-4 pt-4">
                  {provider.methods.map((method, methodIndex) => (
                    <div key={methodIndex} className="flex justify-between items-start p-3 bg-gray-50 rounded-lg">
                      <div className="space-y-1">
                        <h4 className="font-semibold">{method.name}</h4>
                        <p className="text-sm text-gray-600">{method.description}</p>
                      </div>
                      <div className="text-right">
                        <p className="font-semibold">{method.fee}</p>
                        <p className="text-sm text-gray-500">+ 0.85% our fee</p>
                      </div>
                    </div>
                  ))}
                </div>
              </CardContent>
            )}
          </Card>
        ))}
      </div>

      <Card className="bg-white shadow-lg">
        <CardContent className="py-8">
          <div className="grid md:grid-cols-2 gap-8">
            <div>
              <h3 className="text-lg font-bold mb-4">What's Included</h3>
              <ul className="space-y-3">
                {[
                  'No monthly fees',
                  'No setup fees',
                  'No hidden charges',
                  'Instant settlement',
                  'South African payment methods',
                  '24/7 support'
                ].map((feature, index) => (
                  <li key={index} className="flex items-center">
                    <Check className="h-5 w-5 text-green-500 mr-3" />
                    <span className="text-gray-700">{feature}</span>
                  </li>
                ))}
              </ul>
            </div>
            <div>
              <h3 className="text-lg font-bold mb-4">Our Promise</h3>
              <p className="text-gray-600">
                We keep things simple and transparent. Our fee is consistently 0.85% across all payment methods. 
                The processing fees go directly to our payment partners to handle the 
                secure processing of your transactions.
              </p>
            </div>
          </div>
        </CardContent>
      </Card>

      <div className="mt-8 text-center text-sm text-gray-500">
        <p>Contact us for volume pricing on transactions over R100,000 monthly.</p>
      </div>
    </div>
  );
};

export default PricingPage;