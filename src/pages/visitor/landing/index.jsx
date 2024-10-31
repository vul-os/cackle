import React from 'react';
import Header from './header';
import Footer from './footer';
import Hero from './hero';
import { Card, CardContent } from "@/components/ui/card";
import { ChevronRight, Calendar, MapPin } from 'lucide-react';
import { FEATURED_EVENTS, UPCOMING_EVENTS, CATEGORIES, ARTISTS } from './dummy';

const SectionHeader = ({ title, subtitle, seeAllLink }) => (
  <div className="flex justify-between items-center mb-8">
    <div>
      <h2 className="text-3xl font-extrabold bg-gradient-to-r from-[#800020] to-[#FF4D6A] bg-clip-text text-transparent drop-shadow-sm">
        {title}
      </h2>
      <p className="text-black dark:text-black mt-1 font-medium">
        {subtitle}
      </p>
    </div>
    
    {seeAllLink && (
      <a 
        href={seeAllLink} 
        className="flex items-center text-[#FF4D6A] hover:text-[#FF6B83] transition-colors duration-300 font-semibold"
      >
        <span className="mr-2">See All</span>
        <ChevronRight className="h-4 w-4" />
      </a>
    )}
  </div>
);

function HowlerLandingPage() {
  return (
    <div className="min-h-screen flex flex-col bg-gradient-to-r from-gray-200 via-white to-gray-200 dark:from-navy-950 dark:via-slate-950 dark:to-navy-950 transition-colors duration-200">
      {/* Background Gradients */}
      <div className="fixed inset-0 -z-10 overflow-hidden">
        <div className="absolute top-0 left-0 w-full h-full bg-[radial-gradient(ellipse_at_top,_var(--tw-gradient-stops))] from-gray-900 via-gray-950 to-black dark:from-navy-900 dark:via-navy-950 dark:to-slate-950 transition-colors duration-200" />
        
        <div className="absolute top-0 -left-48 w-96 h-96 bg-gray-800/40 dark:bg-navy-800/20 rounded-full blur-3xl animate-pulse" />
        
        <div 
          className="absolute top-1/4 -right-48 w-96 h-96 bg-gray-900/40 dark:bg-navy-700/20 rounded-full blur-3xl animate-pulse" 
          style={{ animationDelay: '1s' }}
        />
        
        <div 
          className="absolute bottom-0 left-1/3 w-96 h-96 bg-black/30 dark:bg-navy-600/10 rounded-full blur-3xl animate-pulse" 
          style={{ animationDelay: '2s' }}
        />
        
        <div className="absolute inset-0 bg-[url('/api/placeholder/8/8')] opacity-[0.02] bg-repeat" />
        <div className="absolute inset-0 backdrop-blur-[100px]" />
      </div>

      <Header />

      <main className="flex-grow relative">
        <Hero />

        {/* Featured Events Section */}
        <section id="section-featured-events" className="py-16 relative">
          {/* Featured events content */}
        </section>

        {/* Upcoming Events Section */}
        <section id="section-upcoming" className="py-16 relative">
          <div className="container mx-auto px-4">
            <SectionHeader 
              title="Upcoming Events" 
              subtitle="Events happening soon"
              seeAllLink="/upcoming"
            />
            
            <div className="grid grid-cols-1 md:grid-cols-3 gap-6">
              {UPCOMING_EVENTS.map(event => (
                <Card 
                  key={event.id} 
                  className="group overflow-hidden bg-gradient-to-br from-black/90 to-black/95 backdrop-blur-sm shadow-lg hover:shadow-2xl transition-all duration-300 transform hover:-translate-y-2 rounded-xl border-0"
                >
                  <div className="aspect-[16/9] relative overflow-hidden">
                    <img
                      src={event.image}
                      alt={event.title}
                      className="w-full h-full object-cover transition-transform duration-300 group-hover:scale-105"
                    />
                    
                    <div className="absolute inset-0 bg-gradient-to-t from-black/90 via-black/50 to-transparent" />
                    
                    <div className="absolute top-4 right-4 px-3 py-1 bg-gradient-to-r from-[#800020] to-[#FF4D6A] rounded-full text-white text-sm font-semibold drop-shadow-lg">
                      {event.price}
                    </div>
                  </div>
                  
                  <CardContent className="p-6 relative bg-gradient-to-br from-[#800020]/10 to-transparent">
                    <h3 className="font-bold text-2xl mb-4 text-white group-hover:text-[#FF4D6A] transition-colors duration-200 drop-shadow-sm tracking-wide">
                      {event.title}
                    </h3>
                    
                    <div className="space-y-3 relative">
                      <div className="flex items-center text-white/90">
                        <MapPin className="w-4 h-4 mr-2 text-[#FF4D6A]" />
                        <span className="text-sm font-medium tracking-wide">
                          {event.venue}
                        </span>
                      </div>
                      
                      <div className="flex items-center text-white/80">
                        <Calendar className="w-4 h-4 mr-2 text-[#FF4D6A]" />
                        <span className="text-sm font-medium tracking-wide">
                          {event.date}
                        </span>
                      </div>
                    </div>

                    <button className="mt-6 w-full bg-gradient-to-r from-[#800020] to-[#FF4D6A] hover:from-[#FF4D6A] hover:to-[#800020] text-white py-2.5 px-4 rounded-lg transition-all duration-200 font-semibold transform hover:scale-[1.02] shadow-lg">
                      Get Tickets
                    </button>
                  </CardContent>
                </Card>
              ))}
            </div>
          </div>
        </section>

        {/* Categories Section */}
        <section id="section-categories" className="py-16 relative">
          <div className="container mx-auto px-4">
            <SectionHeader 
              title="Categories" 
              subtitle="Explore events that match your tastes"
              seeAllLink="/categories"
            />
            
            <div className="grid grid-cols-2 md:grid-cols-4 gap-6">
              {CATEGORIES.map(category => (
                <a
                  key={category.id}
                  href={`/categories/${category.id}`}
                  className="group relative aspect-square rounded-xl overflow-hidden shadow-lg hover:shadow-2xl transition-all duration-300"
                >
                  <img 
                    src={category.image}
                    alt={category.title}
                    className="w-full h-full object-cover transition-transform duration-500 group-hover:scale-110"
                  />
                  
                  <div className="absolute inset-0 bg-gradient-to-t from-[#800020]/80 to-[#FF4D6A]/80 opacity-0 group-hover:opacity-100 transition-opacity duration-300" />
                  
                  <div className="absolute inset-0 bg-gradient-to-t from-black/95 to-black/20 flex flex-col justify-end p-6 transition-colors duration-200">
                    <h3 className="font-bold text-xl text-white drop-shadow-md tracking-wide">
                      {category.title}
                    </h3>
                    <p className="text-sm font-medium text-white/80 drop-shadow">
                      {category.eventCount} Events
                    </p>
                  </div>
                </a>
              ))}
            </div>
          </div>
        </section>

        {/* Artists Section */}
        <section id="section-artists" className="py-16 relative">
          <div className="container mx-auto px-4">
            <SectionHeader 
              title="Trending Artists" 
              subtitle="Keep track of your favourite artists"
              seeAllLink="/artists"
            />
            
            <div className="grid grid-cols-2 md:grid-cols-4 gap-6">
              {ARTISTS.map(artist => (
                <Card 
                  key={artist.id} 
                  className="overflow-hidden bg-gradient-to-br from-black/90 to-black/95 backdrop-blur-sm shadow-lg hover:shadow-2xl transition-all duration-300 transform hover:-translate-y-1 rounded-xl border-0"
                >
                  <div className="aspect-square relative">
                    <img 
                      src={artist.image}
                      alt={artist.name}
                      className="w-full h-full object-cover"
                    />
                    <div className="absolute inset-0 bg-gradient-to-br from-[#800020]/20 to-[#FF4D6A]/20 opacity-0 hover:opacity-100 transition-opacity duration-300" />
                  </div>
                  
                  <CardContent className="p-6 bg-gradient-to-br from-[#800020]/10 to-transparent">
                    <h3 className="font-bold text-xl text-white tracking-wide drop-shadow-sm">
                      {artist.name}
                    </h3>
                    <p className="text-white/80 text-sm font-medium mt-1">
                      {artist.eventCount} Events
                    </p>
                  </CardContent>
                </Card>
              ))}
            </div>
          </div>
        </section>
      </main>

      <Footer />
    </div>
  );
}

export default HowlerLandingPage;