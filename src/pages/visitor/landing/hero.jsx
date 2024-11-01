import React, { useState, useEffect, useCallback, memo } from 'react';
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Search, X } from 'lucide-react';
import TabNavigation from './tabs';
import { SearchOverlay } from './search';

const HERO_TABS = [
  { id: 'section-featured-events', label: 'Featured Events' },
  { id: 'section-upcoming', label: 'Upcoming' },
  { id: 'section-categories', label: 'Categories' },
  { id: 'section-artists', label: 'Artists' }
];

const backgroundImages = [
  { src: '/images/jog.jpg', style: { top: '5%', right: '10%', width: '400px', height: '300px', transform: 'rotate(10deg)' }},
  { src: '/images/jog.jpg', style: { top: '20%', left: '5%', width: '300px', height: '200px', transform: 'rotate(-5deg)' }},
  { src: '/images/quiz.jpg', style: { bottom: '15%', right: '15%', width: '350px', height: '250px', transform: 'rotate(8deg)' }},
  { src: '/images/racing.jpeg', style: { bottom: '25%', left: '20%', width: '280px', height: '210px', transform: 'rotate(-12deg)' }},
  { src: '/images/yoga.jpg', style: { top: '40%', right: '25%', width: '320px', height: '240px', transform: 'rotate(15deg)' }}
];

const Firework = memo(() => {
  return (
    <div className="firework">
      {Array.from({ length: 5 }, (_, i) => (
        <div key={i} className="particle-container">
          {Array.from({ length: 24 }, (_, j) => (
            <div key={j} className={`particle particle-${j}`} />
          ))}
        </div>
      ))}
    </div>
  );
});

Firework.displayName = 'Firework';

const BackgroundImage = memo(({ img, index }) => (
  <div
    className="absolute rounded-xl overflow-hidden transition-transform duration-700 ease-in-out hover:scale-105 hidden md:block"
    style={img.style}
  >
    <img
      src={img.src}
      alt={`Background ${index + 1}`}
      className="w-full h-full object-cover"
      loading="lazy"
    />
  </div>
));

BackgroundImage.displayName = 'BackgroundImage';

function Hero() {
  const [isSearchOpen, setIsSearchOpen] = useState(false);
  const [activeTab, setActiveTab] = useState(HERO_TABS[0].id);
  const [isSticky, setIsSticky] = useState(false);
  const [hoveredTab, setHoveredTab] = useState(null);
  const [isMobile, setIsMobile] = useState(false);

  useEffect(() => {
    const checkMobile = () => {
      setIsMobile(window.innerWidth < 768);
    };
    
    checkMobile();
    window.addEventListener('resize', checkMobile);
    
    return () => window.removeEventListener('resize', checkMobile);
  }, []);

  useEffect(() => {
    let timeoutId;
    const handleScroll = () => {
      if (timeoutId) {
        window.cancelAnimationFrame(timeoutId);
      }

      timeoutId = window.requestAnimationFrame(() => {
        const tabsElement = document.getElementById('navigation-tabs');
        if (tabsElement) {
          const headerHeight = 64;
          const tabsPosition = tabsElement.getBoundingClientRect().top + window.scrollY;
          setIsSticky(window.scrollY > tabsPosition - headerHeight);
        }
      });
    };

    window.addEventListener('scroll', handleScroll, { passive: true });
    return () => {
      window.removeEventListener('scroll', handleScroll);
      if (timeoutId) {
        window.cancelAnimationFrame(timeoutId);
      }
    };
  }, []);

  const handleSearchClick = useCallback(() => {
    setIsSearchOpen(true);
  }, []);

  const handleTabClick = useCallback((tabId) => {
    setActiveTab(tabId);
    
    // Always scroll to section, regardless of device type
    const element = document.getElementById(tabId);
    if (element) {
      const headerHeight = 64;
      const tabsHeight = 56;
      const totalOffset = isSticky ? headerHeight + tabsHeight + 16 : headerHeight + 16;
      
      window.scrollTo({
        top: element.offsetTop - totalOffset,
        behavior: 'smooth'
      });
    }
  }, [isSticky]);

  return (
    <>      
      <style jsx>{`
        @keyframes explode {
          0% {
            transform: translate(-50%, -50%) scale(0);
            opacity: 1;
          }
          50% {
            transform: translate(-50%, -50%) scale(0.6);
            opacity: 1;
          }
          100% {
            transform: translate(-50%, -50%) scale(1.2);
            opacity: 0;
          }
        }

        @keyframes particle {
          0% {
            transform: translate(0, 0) scale(1);
            opacity: 1;
          }
          50% {
            transform: translate(calc(var(--tx) * 0.6), calc(var(--ty) * 0.6)) scale(0.8);
            opacity: 0.8;
          }
          100% {
            transform: translate(var(--tx), var(--ty)) scale(0);
            opacity: 0;
          }
        }

        .firework {
          position: absolute;
          top: 50%;
          left: 50%;
          pointer-events: none;
          z-index: 50;
        }

        .particle-container {
          position: absolute;
          top: 0;
          left: 0;
          animation: explode 1.5s ease-out forwards;
        }

        .particle {
          position: absolute;
          top: 0;
          left: 0;
          width: 6px;
          height: 6px;
          border-radius: 50%;
          background: linear-gradient(to right, #ff4d4d, #ff0000);
          animation: particle 1.8s ease-out forwards;
          box-shadow: 0 0 8px #ff4d4d;
        }

        .particle:nth-child(1) { --tx: 80px; --ty: -80px; }
        .particle:nth-child(2) { --tx: 80px; --ty: 80px; }
        .particle:nth-child(3) { --tx: -80px; --ty: -80px; }
        .particle:nth-child(4) { --tx: -80px; --ty: 80px; }
        .particle:nth-child(5) { --tx: 100px; --ty: 0px; }
        .particle:nth-child(6) { --tx: -100px; --ty: 0px; }
        .particle:nth-child(7) { --tx: 0px; --ty: 100px; }
        .particle:nth-child(8) { --tx: 0px; --ty: -100px; }
        .particle:nth-child(9) { --tx: 120px; --ty: -120px; }
        .particle:nth-child(10) { --tx: 120px; --ty: 120px; }
        .particle:nth-child(11) { --tx: -120px; --ty: -120px; }
        .particle:nth-child(12) { --tx: -120px; --ty: 120px; }
        .particle:nth-child(13) { --tx: 150px; --ty: -50px; }
        .particle:nth-child(14) { --tx: 150px; --ty: 50px; }
        .particle:nth-child(15) { --tx: -150px; --ty: -50px; }
        .particle:nth-child(16) { --tx: -150px; --ty: 50px; }
        .particle:nth-child(17) { --tx: 180px; --ty: -180px; }
        .particle:nth-child(18) { --tx: 180px; --ty: 180px; }
        .particle:nth-child(19) { --tx: -180px; --ty: -180px; }
        .particle:nth-child(20) { --tx: -180px; --ty: 180px; }
        .particle:nth-child(21) { --tx: 200px; --ty: -100px; }
        .particle:nth-child(22) { --tx: 200px; --ty: 100px; }
        .particle:nth-child(23) { --tx: -200px; --ty: -100px; }
        .particle:nth-child(24) { --tx: -200px; --ty: 100px; }

        .particle-container:nth-child(2) { animation-delay: 0.2s; }
        .particle-container:nth-child(3) { animation-delay: 0.4s; }
        .particle-container:nth-child(4) { animation-delay: 0.6s; }
        .particle-container:nth-child(5) { animation-delay: 0.8s; }

        @media (hover: none) {
          .particle {
            animation-duration: 1.5s;
          }
          .particle-container {
            animation-duration: 1.2s;
          }
        }

        @media (min-width: 768px) {
          .tabs-container {
            overflow-y: hidden !important;
          }
        }

        @media (max-width: 767px) {
          .tabs-container {
            -webkit-overflow-scrolling: touch;
            scrollbar-width: none;
          }
          .tabs-container::-webkit-scrollbar {
            display: none;
          }
        }
      `}</style>

      <div className="relative min-h-[300px] sm:min-h-[400px] md:min-h-[600px] bg-black dark:bg-slate-950 transition-colors duration-200 mt-16">
        <div className="absolute inset-0 overflow-hidden">
          <div className="absolute inset-0 bg-black dark:bg-slate-950 transition-colors duration-200" />
          
          {backgroundImages.map((img, index) => (
            <BackgroundImage key={index} img={img} index={index} />
          ))}

          <div 
            className="absolute inset-0"
            style={{
              background: 'linear-gradient(to right, rgba(0,0,0,0.9) 0%, rgba(0,0,0,0.7) 35%, rgba(0,0,0,0.4) 65%, rgba(0,0,0,0.2) 100%)'
            }}
          />
        </div>

        <div className="relative z-10">
          <div className="hero-content container mx-auto px-4 md:px-6 pt-8 sm:pt-12 md:pt-24 pb-12 sm:pb-16 md:pb-32">
            <div className="max-w-2xl">
              <h1 className="text-2xl sm:text-3xl md:text-5xl font-bold mb-3 sm:mb-4 md:mb-6 bg-gradient-to-r from-red-400 via-red-500 to-red-600 bg-clip-text text-transparent drop-shadow-lg">
                At the heart of the best events
              </h1>
              <p className="text-base sm:text-lg md:text-xl mb-6 sm:mb-8 md:mb-16 text-white dark:text-slate-200 opacity-90 drop-shadow transition-colors duration-200">
                Less work, more play. Whether you're into online streams, weekend festivals 
                or daytime get-togethers; we have something for you. Find what you're 
                looking for and join the movement.
              </p>

              <Card 
                className="bg-white/90 dark:bg-slate-900/90 backdrop-blur cursor-pointer hover:shadow-lg transition-all duration-200"
                onClick={handleSearchClick}
              >
                <CardContent className="p-2 sm:p-3 md:p-4 flex items-center text-gray-600 dark:text-slate-300">
                  <Search className="h-4 md:h-5 w-4 md:w-5 mr-2 md:mr-3" />
                  <span className="text-xs sm:text-sm md:text-base">Search events, organisers, venues or artists</span>
                </CardContent>
              </Card>
            </div>
          </div>
        </div>
      </div>

      <div className="relative -mt-8" id="navigation-tabs">
        {isSticky && <div style={{ height: '56px' }} />}
        
        <div 
          className={`w-full bg-black border-b border-gray-800 transition-all duration-300 ${
            isSticky ? 'fixed top-16 left-0 right-0 z-40' : ''
          }`}
        >
          <div className="container mx-auto tabs-container">
            <div 
              className={`flex md:grid md:grid-cols-4 w-full ${
                isMobile ? 'min-w-[480px]' : ''
              }`}
            >
              {HERO_TABS.map((tab) => (
                <div 
                  key={tab.id} 
                  className="relative flex-shrink-0 md:flex-shrink"
                >
                  <button
                    onClick={() => handleTabClick(tab.id)}
                    onMouseEnter={() => !isMobile && setHoveredTab(tab.id)}
                    onMouseLeave={() => !isMobile && setHoveredTab(null)}
                    onTouchStart={() => isMobile && setHoveredTab(tab.id)}
                    onTouchEnd={() => isMobile && setHoveredTab(null)}
                    className={`
                      w-full 
                      px-2 sm:px-4 md:px-6 
                      py-2 sm:py-3 md:py-4 
                      text-xs sm:text-sm md:text-lg 
                      font-medium 
                      whitespace-nowrap 
                      transition-all 
                      duration-300 
                      relative
                      ${
                        activeTab === tab.id 
                          ? 'text-white border-b-2 border-red-500 scale-105 transform' 
                          : 'text-gray-400 hover:text-white hover:scale-105 transform'
                      }
                      hover:bg-gradient-to-b 
                      hover:from-transparent 
                      hover:to-red-500/10
                    `}
                  >
                    {tab.label}
                    {activeTab === tab.id && (
                      <span className="absolute bottom-0 left-0 w-full h-0.5 bg-gradient-to-r from-red-400 to-red-600" />
                    )}
                  </button>
                  {hoveredTab === tab.id && <Firework />}
                </div>
              ))}
            </div>
          </div>
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
}

export default Hero;