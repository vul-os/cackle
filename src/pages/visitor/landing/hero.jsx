import React, { useState } from 'react';
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Search, X } from 'lucide-react';
import TabNavigation from './tabs';
import { SearchOverlay } from './search';

export const HERO_TABS = [
  { id: 'featured-events', label: 'Featured Events' },
  { id: 'upcoming', label: 'Upcoming' },
  { id: 'categories', label: 'Categories' },
  { id: 'artists', label: 'Artists' }
];

const backgroundImages = [
  { src: '/images/jog.jpg', style: { top: '5%', right: '10%', width: '400px', height: '300px', transform: 'rotate(10deg)' }},
  { src: '/images/jog.jpg', style: { top: '20%', left: '5%', width: '300px', height: '200px', transform: 'rotate(-5deg)' }},
  { src: '/images/quiz.jpg', style: { bottom: '15%', right: '15%', width: '350px', height: '250px', transform: 'rotate(8deg)' }},
  { src: '/images/racing.jpeg', style: { bottom: '25%', left: '20%', width: '280px', height: '210px', transform: 'rotate(-12deg)' }},
  { src: '/images/yoga.jpg', style: { top: '40%', right: '25%', width: '320px', height: '240px', transform: 'rotate(15deg)' }}
];


const Hero = () => {
  const [isSearchOpen, setIsSearchOpen] = useState(false);
  const [activeTab, setActiveTab] = useState(HERO_TABS[0].id);

  const handleSearchClick = () => {
    setIsSearchOpen(true);
  };

  return (
    <>      
      <div className="relative min-h-[600px] bg-black">
        <div className="absolute inset-0 overflow-hidden">
          <div className="absolute inset-0 bg-black" />
          
          {backgroundImages.map((img, index) => (
            <div
              key={index}
              className="absolute rounded-xl overflow-hidden transition-transform duration-700 ease-in-out hover:scale-105"
              style={img.style}
            >
              <img
                src={img.src}
                alt={`Background ${index + 1}`}
                className="w-full h-full object-cover"
              />
            </div>
          ))}

          <div 
            className="absolute inset-0"
            style={{
              background: 'linear-gradient(to right, rgba(0,0,0,0.9) 0%, rgba(0,0,0,0.7) 35%, rgba(0,0,0,0.4) 65%, rgba(0,0,0,0.2) 100%)'
            }}
          />
        </div>

        <div className="relative z-10">
          <div className="container mx-auto px-4 pt-24 pb-32">
            <div className="max-w-2xl">
              <h1 className="text-4xl md:text-5xl font-bold mb-6 bg-gradient-to-r from-purple-400 via-purple-500 to-purple-600 bg-clip-text text-transparent drop-shadow-lg">
                At the heart of the best events
              </h1>
              <p className="text-xl mb-12 text-white opacity-90 drop-shadow">
                Less work, more play. Whether you're into online streams, weekend festivals 
                or daytime get-togethers; we have something for you. Find what you're 
                looking for and join the movement.
              </p>

              <Card 
                className="bg-white/90 backdrop-blur cursor-pointer hover:shadow-lg transition-shadow"
                onClick={handleSearchClick}
              >
                <CardContent className="p-4 flex items-center text-gray-600">
                  <Search className="h-5 w-5 mr-3" />
                  <span>Search events, organisers, venues or artists</span>
                </CardContent>
              </Card>
            </div>
          </div>

          <TabNavigation 
            tabs={HERO_TABS}
            activeTab={activeTab}
            onTabChange={setActiveTab}
          />
        </div>
      </div>

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