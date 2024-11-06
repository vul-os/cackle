import React, { useState, useEffect } from 'react';
import { Calculator, Percent, CreditCard as CreditCardIcon, Banknote as BanknoteIcon, Wallet as WalletIcon } from 'lucide-react';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';

const PaymentCalculator = () => {
  const [numTickets, setNumTickets] = useState(100);
  const [ticketPrice, setTicketPrice] = useState(100);
  const [includeVat, setIncludeVat] = useState(true);
  const [distribution, setDistribution] = useState({
    card: 90,
    cash: 8,
    payshap: 2
  });
  
  // Configurable constants
  const VAT_RATE = 0.15; // 15% VAT
  const OUR_FEE_RATE = 0.0085; // 0.85%

  // Payment method configurations
  const paymentMethods = {
    card: {
      name: 'Card Payments',
      icon: <CreditCardIcon className="h-4 w-4 text-red-600" />,
      feeCalculation: (tickets, price) => (tickets * price) * 0.029 + (tickets * 1)
    },
    cash: {
      name: 'Cash Payments',
      icon: <BanknoteIcon className="h-4 w-4 text-red-600" />,
      feeCalculation: (tickets, price) => (tickets * price) * 0.039 + (tickets * 6)
    },
    payshap: {
      name: 'PayShap',
      icon: <WalletIcon className="h-4 w-4 text-red-600" />,
      feeCalculation: (tickets) => tickets * 7.5
    }
  };

  // Handle distribution changes
  const handleDistributionChange = (method, value) => {
    const newValue = Math.max(0, Math.min(100, parseInt(value) || 0));
    const oldValue = distribution[method];
    const diff = newValue - oldValue;
    
    // Calculate remaining methods' total
    const others = Object.entries(distribution)
      .filter(([key]) => key !== method)
      .map(([key, val]) => ({ key, val }));
    
    const othersTotal = others.reduce((sum, { val }) => sum + val, 0);
    
    if (othersTotal === 0 && diff < 0) {
      // If reducing and others are 0, distribute evenly
      const eachShare = Math.abs(diff) / others.length;
      const newDist = { ...distribution, [method]: newValue };
      others.forEach(({ key }) => {
        newDist[key] = eachShare;
      });
      setDistribution(newDist);
    } else {
      // Proportionally adjust other values
      const newDist = { ...distribution, [method]: newValue };
      const scale = (100 - newValue) / othersTotal;
      
      others.forEach(({ key, val }) => {
        newDist[key] = Math.round(val * scale);
      });
      
      // Handle rounding errors
      const total = Object.values(newDist).reduce((sum, val) => sum + val, 0);
      if (total !== 100) {
        const lastKey = others[others.length - 1].key;
        newDist[lastKey] += 100 - total;
      }
      
      setDistribution(newDist);
    }
  };

  // Calculate tickets and fees based on distribution
  const calculateDistribution = () => {
    return Object.entries(distribution).map(([method, percentage]) => {
      const tickets = Math.round(numTickets * (percentage / 100));
      const methodConfig = paymentMethods[method];
      const fees = methodConfig.feeCalculation(tickets, ticketPrice);
      return {
        id: method,
        name: methodConfig.name,
        icon: methodConfig.icon,
        percentage: percentage / 100,
        tickets,
        fees
      };
    });
  };

  // Calculate all fees
  const calculateFees = () => {
    const distributionData = calculateDistribution();
    const ourFee = numTickets * ticketPrice * OUR_FEE_RATE;
    const methodFees = distributionData.reduce((sum, method) => sum + method.fees, 0);
    const subtotal = ourFee + methodFees;
    const vat = includeVat ? subtotal * VAT_RATE : 0;
    
    return {
      ourFee,
      methodFees: distributionData.map(method => ({
        name: method.name,
        fees: method.fees
      })),
      subtotal,
      vat,
      total: subtotal + vat
    };
  };

  const distributionData = calculateDistribution();
  const fees = calculateFees();
  const maxTickets = Math.max(...distributionData.map(method => method.tickets));

  return (
    <div className="mt-16">
      <div className="text-center mb-12">
        <span className="text-red-600 font-semibold text-sm tracking-wider uppercase mb-4 block">
          Fee Calculator
        </span>
        <h2 className="text-3xl font-bold mb-4 bg-gradient-to-r from-red-600 to-red-800 bg-clip-text text-transparent">
          Calculate Your Fees
        </h2>
        <p className="text-gray-600 max-w-2xl mx-auto">
          Estimate your total costs with our transparent fee calculator
        </p>
      </div>

      <div className="grid md:grid-cols-2 gap-8">
        {/* Input Card */}
        <Card className="bg-white/80 backdrop-blur-sm border-0">
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <Calculator className="h-5 w-5 text-red-600" />
              Event Details
            </CardTitle>
          </CardHeader>
          <CardContent className="space-y-6">
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-2">
                Number of Tickets
              </label>
              <input
                type="number"
                value={numTickets}
                onChange={(e) => setNumTickets(Math.max(0, parseInt(e.target.value) || 0))}
                className="w-full px-4 py-2 border border-gray-300 rounded-md focus:ring-red-500 focus:border-red-500"
              />
            </div>

            <div>
              <label className="block text-sm font-medium text-gray-700 mb-2">
                Ticket Price (R)
              </label>
              <input
                type="number"
                value={ticketPrice}
                onChange={(e) => setTicketPrice(Math.max(0, parseInt(e.target.value) || 0))}
                className="w-full px-4 py-2 border border-gray-300 rounded-md focus:ring-red-500 focus:border-red-500"
              />
            </div>

            <div className="flex items-center">
              <input
                type="checkbox"
                id="includeVat"
                checked={includeVat}
                onChange={(e) => setIncludeVat(e.target.checked)}
                className="h-4 w-4 text-red-600 focus:ring-red-500 border-gray-300 rounded"
              />
              <label htmlFor="includeVat" className="ml-2 block text-sm text-gray-700">
                Include VAT (15%)
              </label>
            </div>
          </CardContent>
        </Card>

        {/* Payment Distribution Card */}
        <Card className="bg-white/80 backdrop-blur-sm border-0">
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <Percent className="h-5 w-5 text-red-600" />
              Payment Distribution
            </CardTitle>
          </CardHeader>
          <CardContent className="space-y-6">
            {Object.entries(distribution).map(([method, percentage]) => {
              const methodConfig = paymentMethods[method];
              return (
                <div key={method} className="space-y-2">
                  <div className="flex items-center justify-between">
                    <div className="flex items-center gap-2">
                      {methodConfig.icon}
                      <span className="text-sm font-medium">
                        {methodConfig.name}
                      </span>
                    </div>
                    <span className="text-sm font-bold">{percentage}%</span>
                  </div>
                  <input
                    type="range"
                    min="0"
                    max="100"
                    value={percentage}
                    onChange={(e) => handleDistributionChange(method, e.target.value)}
                    className="w-full h-2 bg-gray-200 rounded-lg appearance-none cursor-pointer accent-red-600"
                  />
                  <div className="text-sm text-gray-500 text-right">
                    {Math.round(numTickets * (percentage / 100))} tickets
                  </div>
                </div>
              );
            })}
          </CardContent>
        </Card>
      </div>

      {/* Fee Breakdown Card */}
      <Card className="mt-8 bg-white/80 backdrop-blur-sm border-0">
        <CardHeader>
          <CardTitle>Fee Breakdown</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="space-y-4">
            <div className="grid grid-cols-2 gap-4">
              <div className="p-4 bg-red-50 rounded-lg">
                <h4 className="text-sm font-medium text-gray-600 mb-1">Our Fee (0.85%)</h4>
                <p className="text-2xl font-bold text-red-600">
                  R{fees.ourFee.toFixed(2)}
                </p>
              </div>
              {fees.methodFees.map((methodFee, index) => (
                <div key={index} className="p-4 bg-gray-50 rounded-lg">
                  <h4 className="text-sm font-medium text-gray-600 mb-1">{methodFee.name} Fees</h4>
                  <p className="text-2xl font-bold text-gray-900">
                    R{methodFee.fees.toFixed(2)}
                  </p>
                </div>
              ))}
            </div>

            <div className="pt-4 border-t border-gray-200">
              <div className="flex justify-between items-center mb-2">
                <span className="text-gray-600">Subtotal</span>
                <span className="font-bold">R{fees.subtotal.toFixed(2)}</span>
              </div>
              <div className="flex justify-between items-center mb-2">
                <span className="text-gray-600">VAT (15%)</span>
                <span className="font-bold">R{fees.vat.toFixed(2)}</span>
              </div>
              <div className="flex justify-between items-center pt-2 border-t border-gray-200">
                <span className="text-lg font-bold text-gray-900">Total Fees</span>
                <span className="text-lg font-bold text-red-600">R{fees.total.toFixed(2)}</span>
              </div>
            </div>
          </div>
        </CardContent>
      </Card>

      {/* Note Section */}
      <div className="mt-8 text-center text-sm text-gray-600">
        <p>All fees are calculated excluding VAT unless specified otherwise.</p>
        <p>Actual fees may vary based on final transaction volumes and payment methods used.</p>
      </div>
    </div>
  );
};

export default PaymentCalculator;