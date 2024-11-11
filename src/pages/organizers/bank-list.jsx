import React, { useState, useEffect } from 'react';
import { supabase } from '@/services/supabaseClient';

const BankListPage = () => {
  const [banks, setBanks] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);

  useEffect(() => {
    fetchBanks();
  }, []);

  const fetchBanks = async () => {
    try {
      const { data: { session } } = await supabase.auth.getSession();

      // Replace with your Supabase function URL
      const response = await fetch(
        'REDACTED_SUPABASE_URL/functions/v1/list-banks-paystack',
        {
            headers: {
                'Authorization': `Bearer ${session.access_token}`,
            }
        }
      );

      if (!response.ok) {
        throw new Error('Failed to fetch banks');
      }

      const data = await response.json();
      setBanks(data.data);
      setLoading(false);
    } catch (err) {
      setError(err.message);
      setLoading(false);
    }
  };

  if (loading) {
    return (
      <div className="min-h-screen bg-gray-50 p-8">
        <div className="text-center">
          <div className="animate-spin h-8 w-8 border-4 border-blue-500 border-t-transparent rounded-full mx-auto"></div>
          <p className="mt-2 text-gray-600">Loading banks...</p>
        </div>
      </div>
    );
  }

  if (error) {
    return (
      <div className="min-h-screen bg-gray-50 p-8">
        <div className="max-w-2xl mx-auto">
          <div className="bg-red-50 border border-red-200 rounded-lg p-4 text-red-700">
            <p>Error: {error}</p>
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className="min-h-screen bg-gray-50 p-8">
      <div className="max-w-2xl mx-auto">
        <h1 className="text-2xl font-bold mb-6">South African Banks</h1>
        
        <div className="bg-white shadow-sm rounded-lg overflow-hidden">
          <div className="grid grid-cols-3 gap-4 p-4 bg-gray-100 font-medium">
            <div>Bank Name</div>
            <div>Code</div>
            <div>Type</div>
          </div>
          
          <div className="divide-y divide-gray-200">
            {banks.map((bank) => (
              <div 
                key={bank.id} 
                className="grid grid-cols-3 gap-4 p-4 hover:bg-gray-50 transition-colors"
              >
                <div className="text-gray-900">{bank.name}</div>
                <div className="text-gray-600">{bank.code}</div>
                <div className="text-gray-600">
                  <span className={`inline-flex items-center px-2 py-1 rounded-full text-xs font-medium ${
                    bank.type === 'nuban' 
                      ? 'bg-green-100 text-green-800' 
                      : 'bg-blue-100 text-blue-800'
                  }`}>
                    {bank.type}
                  </span>
                </div>
              </div>
            ))}
          </div>
        </div>
        
        <div className="mt-4 text-sm text-gray-500">
          Total banks: {banks.length}
        </div>
      </div>
    </div>
  );
};

export default BankListPage;