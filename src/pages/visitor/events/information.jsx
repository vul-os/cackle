import React from 'react';
import {
  Card,
  CardContent,
} from "@/components/ui/card";
import { Info } from 'lucide-react';
import ProcessedText from './processed-text';

const GradientText = ({ children, className = "" }) => (
  <span className={`bg-gradient-to-r from-red-300 to-red-400 text-transparent bg-clip-text ${className}`}>
    {children}
  </span>
);

const InformationSection = ({ information, policyInfo }) => (
  <div>
    <h2 className="text-3xl font-bold mb-6 flex items-center gap-3">
      <GradientText>Event Information</GradientText>
      <Info className="h-6 w-6 text-red-300" />
    </h2>
    
    <ProcessedText 
      content={information} 
      className="text-gray-700 dark:text-gray-200 mb-8" 
    />
    
    {policyInfo && typeof policyInfo === 'object' && (
      <div className="space-y-6">
        {Object.entries(policyInfo).map(([category, items]) => (
          <div 
            key={category} 
            className="border-b border-red-300/20 last:border-0 pb-6 last:pb-0"
          >
            <h3 className="text-lg font-medium mb-4 text-gray-900 dark:text-white">
              {category}
            </h3>
            <ul className="space-y-3">
              {Array.isArray(items) && items.map((item, index) => (
                <li key={index} className="flex items-start gap-3">
                  <Info className="h-4 w-4 text-red-300 mt-1 flex-shrink-0" />
                  <span className="text-gray-600 dark:text-gray-200">
                    {item}
                  </span>
                </li>
              ))}
            </ul>
          </div>
        ))}
      </div>
    )}
  </div>
);

export default InformationSection;