import React from 'react';
import Header from './header';
import Footer from './footer';
import Hero from './hero';
import { Card, CardContent } from "@/components/ui/card";
import { ChevronRight } from 'lucide-react';
import {FEATURED_EVENTS, UPCOMING_EVENTS, CATEGORIES, ARTISTS} from './dummy'

const SectionHeader = ({ title, subtitle, seeAllLink }) => (
  <div className="flex justify-between items-center mb-8">
    <div>
      <h2 className="text-3xl font-bold bg-gradient-to-r from-[#536BFF] to-[#BE5AE6] bg-clip-text text-transparent">{title}</h2>
      <p className="text-zinc-400 mt-1">{subtitle}</p>
    </div>
    {seeAllLink && (
      <a href={seeAllLink} className="flex items-center text-[#536BFF] hover:text-[#BE5AE6] transition-colors duration-300">
        <span className="mr-2">See All</span>
        <ChevronRight className="h-4 w-4" />
      </a>
    )}
  </div>
);

export default function HowlerLandingPage() {
  return (
    <div className="min-h-screen flex flex-col bg-white">
      <div className="fixed inset-0 -z-10 overflow-hidden">
        <div className="absolute top-0 left-0 w-full h-full bg-[radial-gradient(ellipse_at_top,_var(--tw-gradient-stops))] from-black via-zinc-900 to-black"/>
        <div className="absolute top-0 -left-48 w-96 h-96 bg-[#536BFF]/10 rounded-full blur-3xl animate-pulse"/>
        <div className="absolute top-1/4 -right-48 w-96 h-96 bg-[#BE5AE6]/10 rounded-full blur-3xl animate-pulse" style={{ animationDelay: '1s' }}/>
        <div className="absolute bottom-0 left-1/3 w-96 h-96 bg-[#536BFF]/5 rounded-full blur-3xl animate-pulse" style={{ animationDelay: '2s' }}/>
        <div className="absolute inset-0 bg-[url('/api/placeholder/8/8')] opacity-[0.02] bg-repeat"/>
        <div className="absolute inset-0 backdrop-blur-[100px]"/>
      </div>
      
      <Header />
      
      <main className="flex-grow relative">
        <Hero />

        <section id="section-featured-events" className="py-16 relative">
          <div className="container mx-auto px-4">
            <SectionHeader 
              title="Featured Events" 
              subtitle="Our Favourite Picks" 
            />
            
            <div className="grid grid-cols-1 md:grid-cols-12 gap-6">
              <div className="md:col-span-8">
                <a href={FEATURED_EVENTS[0].link} className="block aspect-[16/9] relative rounded-xl overflow-hidden shadow-lg hover:shadow-2xl transition-all duration-300 transform hover:-translate-y-1">
                  <img 
                    src={FEATURED_EVENTS[0].image} 
                    alt={FEATURED_EVENTS[0].title}
                    className="w-full h-full object-cover"
                  />
                  <div className="absolute inset-0 bg-gradient-to-t from-black/80 to-transparent" />
                </a>
              </div>
              <div className="md:col-span-4 grid grid-cols-1 gap-4">
                {FEATURED_EVENTS.slice(1).map(event => (
                  <a 
                    key={event.id}
                    href={event.link}
                    className="block aspect-[16/9] relative rounded-xl overflow-hidden shadow-lg hover:shadow-2xl transition-all duration-300 transform hover:-translate-y-1"
                  >
                    <img 
                      src={event.image}
                      alt={event.title}
                      className="w-full h-full object-cover"
                    />
                    <div className="absolute inset-0 bg-gradient-to-t from-black/80 to-transparent" />
                  </a>
                ))}
              </div>
            </div>
          </div>
        </section>

        <section id="section-upcoming" className="py-16 relative">
          <div className="container mx-auto px-4">
            <SectionHeader 
              title="Upcoming Events" 
              subtitle="Events happening soon"
              seeAllLink="/upcoming"
            />
            
            <div className="grid grid-cols-1 md:grid-cols-3 gap-6">
              {UPCOMING_EVENTS.map(event => (
                <Card key={event.id} className="overflow-hidden bg-zinc-900/50 backdrop-blur-sm shadow-lg hover:shadow-2xl transition-all duration-300 transform hover:-translate-y-1 rounded-xl border-0">
                  <div className="aspect-[16/9] relative">
                    <img 
                      src={event.image}
                      alt={event.title}
                      className="w-full h-full object-cover"
                    />
                    <div className="absolute inset-0 bg-gradient-to-t from-black/80 to-transparent" />
                  </div>
                  <CardContent className="p-6">
                    <h3 className="font-bold mb-2 text-xl text-zinc-100">{event.title}</h3>
                    <p className="text-zinc-300 text-sm mb-2">{event.venue}</p>
                    <p className="text-zinc-400 text-sm mb-3">{event.date}</p>
                    <p className="text-[#536BFF] font-semibold">{event.price}</p>
                  </CardContent>
                </Card>
              ))}
            </div>
          </div>
        </section>

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
                  <div className="absolute inset-0 bg-gradient-to-t from-[#536BFF]/80 to-[#BE5AE6]/80 opacity-0 group-hover:opacity-100 transition-opacity duration-300" />
                  <div className="absolute inset-0 bg-gradient-to-t from-black/90 to-black/20 flex flex-col justify-end p-6">
                    <h3 className="font-bold text-xl text-white">{category.title}</h3>
                    <p className="text-sm text-zinc-300">{category.eventCount} Events</p>
                  </div>
                </a>
              ))}
            </div>
          </div>
        </section>

        <section id="section-artists" className="py-16 relative">
          <div className="container mx-auto px-4">
            <SectionHeader 
              title="Trending Artists" 
              subtitle="Keep track of your favourite artists"
              seeAllLink="/artists"
            />
            
            <div className="grid grid-cols-2 md:grid-cols-4 gap-6">
              {ARTISTS.map(artist => (
                <Card key={artist.id} className="overflow-hidden bg-zinc-900/50 backdrop-blur-sm shadow-lg hover:shadow-2xl transition-all duration-300 transform hover:-translate-y-1 rounded-xl border-0">
                  <div className="aspect-square relative">
                    <img 
                      src={artist.image}
                      alt={artist.name}
                      className="w-full h-full object-cover"
                    />
                    <div className="absolute inset-0 bg-gradient-to-br from-[#536BFF]/20 to-[#BE5AE6]/20 opacity-0 hover:opacity-100 transition-opacity duration-300" />
                  </div>
                  <CardContent className="p-6">
                    <h3 className="font-bold text-xl text-zinc-100">{artist.name}</h3>
                    <p className="text-zinc-300 text-sm">{artist.eventCount} Events</p>
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