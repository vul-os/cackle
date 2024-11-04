// pages/EventPage.jsx
import React from 'react';
import {
  Card,
  CardContent,
} from "@/components/ui/card";
import { Info } from 'lucide-react';

import ProcessedText from './processed-text';

const InformationSection = ({ information, policyInfo }) => (
    <Card className="border-none bg-white/5 backdrop-blur-lg hover:bg-white/10 transition-colors duration-300">
      <CardContent className="p-8">
        <h2 className="text-2xl font-bold text-white mb-6">Information</h2>
        <ProcessedText content={information} className="mb-8" />
        
        {policyInfo && typeof policyInfo === 'object' && (
          <div className="space-y-6">
            {Object.entries(policyInfo).map(([category, items]) => (
              <div key={category} className="border-b border-white/10 last:border-0 pb-6 last:pb-0">
                <h3 className="text-lg font-medium mb-4 text-white">{category}</h3>
                <ul className="space-y-3">
                  {Array.isArray(items) && items.map((item, index) => (
                    <li key={index} className="flex items-start gap-3">
                      <Info className="h-4 w-4 text-[#880424] mt-1 flex-shrink-0" />
                      <span className="text-gray-100">{item}</span>
                    </li>
                  ))}
                </ul>
              </div>
            ))}
          </div>
        )}
      </CardContent>
    </Card>
  );
  
  export default InformationSection