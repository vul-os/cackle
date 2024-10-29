import React, { useState } from 'react';
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Search, X } from 'lucide-react';

const tabs = [
  { id: 'featured-events', label: 'Featured Events' },
  { id: 'upcoming', label: 'Upcoming' },
  { id: 'categories', label: 'Categories' },
  { id: 'artists', label: 'Artists' }
];

const SearchOverlay = ({ isOpen, onClose, onSearch }) => {
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

const Hero = () => {
  const [isSearchOpen, setIsSearchOpen] = useState(false);
  const [activeTab, setActiveTab] = useState('featured-events');

  const handleSearchClick = () => {
    setIsSearchOpen(true);
  };

  return (
    <>      
      <div className="relative bg-gradient-to-br pt-10 from-[#ec0b62] to-[#ad0846] text-white">
        {/* Main Hero Content */}
        <div className="container mx-auto px-4 pt-16 pb-32">
          <div className="max-w-2xl">
            <h1 className="text-4xl md:text-5xl font-bold mb-6">
              At the heart of the best events
            </h1>
            <p className="text-xl mb-12 opacity-90">
              Less work, more play. Whether you're into online streams, weekend festivals 
              or daytime get-togethers; we have something for you. Find what you're 
              looking for and join the movement.
            </p>

            {/* Search Bar */}
            <Card 
              className="bg-white cursor-pointer hover:shadow-lg transition-shadow"
              onClick={handleSearchClick}
            >
              <CardContent className="p-4 flex items-center text-gray-600">
                <Search className="h-5 w-5 mr-3" />
                <span>Search events, organisers, venues or artists</span>
              </CardContent>
            </Card>
          </div>
        </div>

        {/* Tabs Navigation */}
        <div className="absolute bottom-0 left-0 right-0 bg-white/10 backdrop-blur-sm">
          <div className="container mx-auto px-4">
            <div className="overflow-x-auto">
              <nav className="flex space-x-8" aria-label="Tabs">
                {tabs.map((tab) => (
                  <a
                    key={tab.id}
                    href={`#section-${tab.id}`}
                    className={`
                      whitespace-nowrap py-4 px-1 border-b-2 font-medium text-sm
                      ${activeTab === tab.id 
                        ? 'border-white text-white' 
                        : 'border-transparent text-white/70 hover:text-white hover:border-white/50'}
                    `}
                    onClick={(e) => {
                      e.preventDefault();
                      setActiveTab(tab.id);
                    }}
                  >
                    {tab.label}
                  </a>
                ))}
              </nav>
            </div>
          </div>
        </div>
      </div>

      {/* Search Overlay */}
      <SearchOverlay 
        isOpen={isSearchOpen}
        onClose={() => setIsSearchOpen(false)}
        onSearch={(query) => {
          console.log('Search query:', query);
        }}
      />
    </>
  );
};

export default Hero;