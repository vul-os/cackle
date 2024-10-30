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
      <h2 className="text-2xl font-bold text-slate-900">{title}</h2>
      <p className="text-slate-600">{subtitle}</p>
    </div>
    {seeAllLink && (
      <a href={seeAllLink} className="flex items-center text-blue-600 hover:text-blue-800">
        <span className="mr-2">See All</span>
        <ChevronRight className="h-4 w-4" />
      </a>
    )}
  </div>
);

export default function HowlerLandingPage() {
  return (
    <div className="min-h-screen flex flex-col bg-slate-50">
      <Header />
      
      <main className="flex-grow">
        {/* Hero Section */}
        <Hero />

        {/* Featured Events Section */}
        <section id="section-featured-events" className="py-12 bg-white">
          <div className="container mx-auto px-4">
            <SectionHeader 
              title="Featured Events" 
              subtitle="Our Favourite Picks" 
            />
            
            <div className="grid grid-cols-1 md:grid-cols-12 gap-6">
              <div className="md:col-span-8">
                <a href={FEATURED_EVENTS[0].link} className="block aspect-[16/9] relative rounded-lg overflow-hidden shadow-md">
                  <img 
                    src={FEATURED_EVENTS[0].image} 
                    alt={FEATURED_EVENTS[0].title}
                    className="w-full h-full object-cover"
                  />
                </a>
              </div>
              <div className="md:col-span-4 grid grid-cols-1 gap-4">
                {FEATURED_EVENTS.slice(1).map(event => (
                  <a 
                    key={event.id}
                    href={event.link}
                    className="block aspect-[16/9] relative rounded-lg overflow-hidden shadow-md"
                  >
                    <img 
                      src={event.image}
                      alt={event.title}
                      className="w-full h-full object-cover"
                    />
                  </a>
                ))}
              </div>
            </div>
          </div>
        </section>

        {/* Upcoming Events Section */}
        <section id="section-upcoming" className="py-12 bg-slate-100">
          <div className="container mx-auto px-4">
            <SectionHeader 
              title="Upcoming Events" 
              subtitle="Events happening soon"
              seeAllLink="/upcoming"
            />
            
            <div className="grid grid-cols-1 md:grid-cols-3 gap-6">
              {UPCOMING_EVENTS.map(event => (
                <Card key={event.id} className="overflow-hidden bg-white shadow-md hover:shadow-lg transition-shadow">
                  <div className="aspect-[16/9] relative">
                    <img 
                      src={event.image}
                      alt={event.title}
                      className="w-full h-full object-cover"
                    />
                  </div>
                  <CardContent className="p-4">
                    <h3 className="font-bold mb-2 text-slate-900">{event.title}</h3>
                    <p className="text-slate-600 text-sm mb-2">{event.venue}</p>
                    <p className="text-slate-500 text-sm mb-2">{event.date}</p>
                    <p className="text-blue-600 font-semibold">{event.price}</p>
                  </CardContent>
                </Card>
              ))}
            </div>
          </div>
        </section>

        {/* Categories Section */}
        <section id="section-categories" className="py-12 bg-white">
          <div className="container mx-auto px-4">
            <SectionHeader 
              title="Categories" 
              subtitle="Explore events that match your tastes"
              seeAllLink="/categories"
            />
            
            <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
              {CATEGORIES.map(category => (
                <a
                  key={category.id}
                  href={`/categories/${category.id}`}
                  className="group relative aspect-square rounded-lg overflow-hidden shadow-md"
                >
                  <img 
                    src={category.image}
                    alt={category.title}
                    className="w-full h-full object-cover transition-transform duration-300 group-hover:scale-105"
                  />
                  <div className="absolute inset-0 bg-slate-900 bg-opacity-50 flex flex-col justify-end p-4 text-slate-50">
                    <h3 className="font-bold">{category.title}</h3>
                    <p className="text-sm text-slate-200">{category.eventCount} Events</p>
                  </div>
                </a>
              ))}
            </div>
          </div>
        </section>

        {/* Artists Section */}
        <section id="section-artists" className="py-12 bg-slate-100">
          <div className="container mx-auto px-4">
            <SectionHeader 
              title="Trending Artists" 
              subtitle="Keep track of your favourite artists"
              seeAllLink="/artists"
            />
            
            <div className="grid grid-cols-2 md:grid-cols-4 gap-6">
              {ARTISTS.map(artist => (
                <Card key={artist.id} className="overflow-hidden bg-white hover:shadow-lg transition-shadow">
                  <div className="aspect-square relative">
                    <img 
                      src={artist.image}
                      alt={artist.name}
                      className="w-full h-full object-cover"
                    />
                  </div>
                  <CardContent className="p-4">
                    <h3 className="font-bold text-slate-900">{artist.name}</h3>
                    <p className="text-slate-600 text-sm">{artist.eventCount} Events</p>
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