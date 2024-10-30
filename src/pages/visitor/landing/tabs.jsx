import React, { useState, useEffect, useRef } from 'react';

const TabNavigation = ({ tabs, activeTab, onTabChange }) => {
  const [indicatorStyle, setIndicatorStyle] = useState({});
  const tabsRef = useRef(new Map());

  useEffect(() => {
    const activeTabElement = tabsRef.current.get(activeTab);
    if (activeTabElement) {
      const { offsetLeft, offsetWidth } = activeTabElement;
      setIndicatorStyle({
        transform: `translateX(${offsetLeft}px)`,
        width: `${offsetWidth}px`,
      });
    }
  }, [activeTab]);

  return (
    <div className="absolute bottom-0 left-0 right-0 bg-black/20 backdrop-blur-sm h-16">
      <div className="container mx-auto px-4 h-full flex items-center">
        <div className="overflow-x-auto overflow-y-hidden w-full">
          <nav className="flex space-x-12 relative items-center min-w-max" aria-label="Tabs">
            {/* Animated indicator */}
            <div 
              className="absolute bottom-0 h-1 bg-white rounded-full transition-all duration-300 ease-in-out"
              style={indicatorStyle}
            >
              {/* Glowing orb */}
              <div className="absolute -top-1.5 left-1/2 -translate-x-1/2 w-4 h-4 bg-white rounded-full animate-pulse" />
            </div>
            
            {tabs.map((tab) => (
              <a
                key={tab.id}
                ref={el => el && tabsRef.current.set(tab.id, el)}
                href={`#section-${tab.id}`}
                className={`
                  group
                  relative
                  whitespace-nowrap px-1 
                  font-bold text-lg tracking-wide
                  transition-all duration-300
                  ${activeTab === tab.id 
                    ? 'text-white' 
                    : 'text-white/70 hover:text-white'}
                `}
                onClick={(e) => {
                  e.preventDefault();
                  onTabChange(tab.id);
                }}
              >
                {/* Text with scale animation */}
                <span className={`
                  inline-block transition-transform duration-300
                  ${activeTab === tab.id 
                    ? 'scale-110 text-shadow-glow' 
                    : 'group-hover:scale-105'}
                  uppercase
                `}>
                  {tab.label}
                </span>
                
                {/* Hover effect - subtle glow */}
                <div className={`
                  absolute inset-0 rounded-lg opacity-0 
                  group-hover:opacity-10
                  transition-opacity duration-300
                  bg-white blur-sm
                `} />
              </a>
            ))}
          </nav>
        </div>
      </div>
    </div>
  );
};

// Add custom CSS for text shadow glow
const style = document.createElement('style');
style.textContent = `
  .text-shadow-glow {
    text-shadow: 0 0 10px rgba(255, 255, 255, 0.5);
  }
`;
document.head.appendChild(style);

export default TabNavigation;