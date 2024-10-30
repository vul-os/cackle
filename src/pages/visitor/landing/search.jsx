import React from 'react';
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { Search, X } from 'lucide-react';

export const SearchOverlay = ({ isOpen, onClose, onSearch }) => {
    if (!isOpen) return null;
  
    return (
      <div className="fixed inset-0 bg-black bg-opacity-50 z-50">
        <div className="bg-white p-4 w-full">
          <div className="container mx-auto">
            <div className="relative">
              <Input
                type="search"
                placeholder="Search events, organisers, venues or artists"
                className="w-full pl-10 pr-4 py-6 text-lg"
                autoFocus
              />
              <Search className="absolute left-3 top-1/2 transform -translate-y-1/2 h-5 w-5 text-gray-400" />
              <Button
                variant="ghost"
                size="sm"
                className="absolute right-2 top-1/2 transform -translate-y-1/2"
                onClick={onClose}
              >
                <X className="h-5 w-5" />
              </Button>
            </div>
          </div>
        </div>
      </div>
    );
  };
  