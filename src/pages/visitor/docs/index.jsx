import React, { useState, useEffect, useRef } from 'react';
import { Card } from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import { Button } from '@/components/ui/button';
import { Moon, Sun, Search, Edit, MessageSquare, ChevronDown, ChevronRight } from 'lucide-react';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import rehypeSlug from 'rehype-slug';
import { cn } from "@/lib/utils";

const navigation = [
  {
    title: "Getting Started",
    pages: [
      { id: "introduction", title: "Introduction", subsections: [
        { id: "quick-start", title: "Quick Start" },
        { id: "installation", title: "Installation" },
        { id: "prerequisites", title: "Prerequisites" }
      ]},
      { id: "configuration", title: "Configuration", subsections: [
        { id: "basic-setup", title: "Basic Setup" },
        { id: "advanced-options", title: "Advanced Options" }
      ]}
    ]
  },
  {
    title: "Core Concepts",
    pages: [
      { id: "architecture", title: "Architecture", subsections: [
        { id: "overview", title: "Overview" },
        { id: "components", title: "Components" }
      ]},
      { id: "routing", title: "Routing", subsections: [
        { id: "basic-routing", title: "Basic Routing" },
        { id: "dynamic-routes", title: "Dynamic Routes" }
      ]}
    ]
  }
];

// Helper function to generate ID from text
const generateId = (text) => {
  return text.toString()
    .toLowerCase()
    .replace(/[^a-z0-9 -]/g, '')
    .replace(/\s+/g, '-')
    .replace(/-+/g, '-');
};

const DocLayout = () => {
  const [isDarkMode, setIsDarkMode] = useState(false);
  const [content, setContent] = useState('');
  const [activePage, setActivePage] = useState('introduction');
  const [expandedSections, setExpandedSections] = useState({});
  const mainContentRef = useRef(null);

  const loadContent = async (pageId) => {
    try {
      const response = await fetch(`/content/${pageId}.md`);
      const text = await response.text();
      setContent(text);
      setActivePage(pageId);
      mainContentRef.current?.scrollTo(0, 0);
    } catch (error) {
      console.error('Failed to load content:', error);
      setContent('# Error Loading Content\nSorry, there was an error loading the content.');
    }
  };

  const scrollToSection = (sectionId) => {
    const element = document.getElementById(sectionId);
    if (element) {
      element.scrollIntoView({ behavior: 'smooth' });
    }
  };

  const toggleSection = (sectionTitle) => {
    setExpandedSections(prev => ({
      ...prev,
      [sectionTitle]: !prev[sectionTitle]
    }));
  };

  useEffect(() => {
    loadContent('introduction');
    setExpandedSections({ 'Getting Started': true });
  }, []);

  const renderNavigation = () => (
    <nav className="space-y-6">
      {navigation.map((section) => (
        <div key={section.title}>
          <button
            onClick={() => toggleSection(section.title)}
            className="flex items-center gap-2 font-bold text-xl mb-4 hover:text-purple-600"
          >
            {expandedSections[section.title] ? 
              <ChevronDown className="h-4 w-4" /> : 
              <ChevronRight className="h-4 w-4" />
            }
            {section.title}
          </button>
          
          {expandedSections[section.title] && (
            <div className="flex flex-col space-y-1 ml-4">
              {section.pages.map((page) => (
                <div key={page.id}>
                  <Button 
                    variant="ghost" 
                    className={`justify-start h-8 w-full ${
                      activePage === page.id 
                        ? 'bg-purple-100 dark:bg-purple-900' 
                        : 'text-purple-600 hover:text-purple-900'
                    }`}
                    onClick={() => loadContent(page.id)}
                  >
                    {page.title}
                  </Button>
                  
                  {activePage === page.id && page.subsections && (
                    <div className="ml-4 mt-1 space-y-1">
                      {page.subsections.map((subsection) => (
                        <Button
                          key={subsection.id}
                          variant="ghost"
                          className="justify-start h-7 w-full text-sm text-gray-600 hover:text-purple-900"
                          onClick={() => scrollToSection(subsection.id)}
                        >
                          {subsection.title}
                        </Button>
                      ))}
                    </div>
                  )}
                </div>
              ))}
            </div>
          )}
        </div>
      ))}
    </nav>
  );

  return (
    <div className={`min-h-screen ${isDarkMode ? 'dark' : ''}`}>
      <header className="sticky top-0 z-50 w-full border-b bg-background/95 backdrop-blur supports-[backdrop-filter]:bg-background/60">
        <div className="container flex h-14 items-center justify-between">
          <div className="flex items-center gap-4">
            <a className="flex items-center space-x-2" href="/">
              <span className="font-bold text-xl">Documentation</span>
            </a>
          </div>

          <div className="flex items-center gap-4">
            <div className="relative w-64">
              <Search className="absolute left-2 top-2.5 h-4 w-4 text-muted-foreground" />
              <Input
                placeholder="Search"
                className="pl-8"
              />
            </div>

            <select className="h-9 rounded-md border px-3">
              <option>English</option>
            </select>
          </div>
        </div>
      </header>

      <div className="container flex">
        <aside className="w-64 pr-8 pt-8 sticky top-14 h-[calc(100vh-3.5rem)] overflow-y-auto">
          {renderNavigation()}
        </aside>

        <main ref={mainContentRef} className="flex-1 min-w-0 py-8">
          <div className="flex">
            <div className="flex-1 pr-16">
              <ReactMarkdown
                remarkPlugins={[remarkGfm]}
                rehypePlugins={[rehypeSlug]}
                components={{
                  h1: ({node, children, ...props}) => {
                    const id = generateId(children[0]);
                    return (
                      <h1 
                        id={id}
                        className="scroll-m-20 text-4xl font-bold tracking-tight mb-4" 
                        {...props}
                      >
                        {children}
                      </h1>
                    );
                  },
                  h2: ({node, children, ...props}) => {
                    const id = generateId(children[0]);
                    return (
                      <h2 
                        id={id}
                        className="scroll-m-20 border-b pb-2 text-3xl font-semibold tracking-tight first:mt-0 mt-12 mb-4"
                        {...props}
                      >
                        {children}
                      </h2>
                    );
                  },
                  h3: ({node, children, ...props}) => {
                    const id = generateId(children[0]);
                    return (
                      <h3 
                        id={id}
                        className="scroll-m-20 text-2xl font-semibold tracking-tight mt-8 mb-4"
                        {...props}
                      >
                        {children}
                      </h3>
                    );
                  },
                  p: ({node, ...props}) => (
                    <p 
                      className="leading-7 [&:not(:first-child)]:mt-6 text-muted-foreground"
                      {...props}
                    />
                  ),
                  ul: ({node, ...props}) => (
                    <ul 
                      className="my-6 ml-6 list-disc [&>li]:mt-2"
                      {...props}
                    />
                  ),
                  ol: ({node, ...props}) => (
                    <ol 
                      className="my-6 ml-6 list-decimal [&>li]:mt-2"
                      {...props}
                    />
                  ),
                  li: ({node, ...props}) => (
                    <li 
                      className="text-muted-foreground"
                      {...props}
                    />
                  ),
                  blockquote: ({node, ...props}) => (
                    <blockquote 
                      className="mt-6 border-l-2 pl-6 italic text-muted-foreground"
                      {...props}
                    />
                  ),
                  img: ({node, src, alt, ...props}) => {
                    // Handle both local and external images
                    const imageSrc = src?.startsWith('http') ? src : `/content/${src}`;
                    return (
                      <img
                        src={imageSrc}
                        alt={alt}
                        className="rounded-md border my-6"
                        {...props}
                      />
                    );
                  },
                  code: ({node, inline, className, children, ...props}) => {
                    if (inline) {
                      return (
                        <code 
                          className="relative rounded bg-muted px-[0.3rem] py-[0.2rem] font-mono text-sm font-semibold"
                          {...props}
                        >
                          {children}
                        </code>
                      );
                    }
                    return (
                      <div className="relative">
                        <pre className="mb-4 mt-6 overflow-x-auto rounded-lg border bg-black py-4 px-4">
                          <code className="relative font-mono text-sm" {...props}>
                            {children}
                          </code>
                        </pre>
                      </div>
                    );
                  },
                  table: ({node, ...props}) => (
                    <div className="my-6 w-full overflow-y-auto">
                      <table className="w-full border-collapse" {...props} />
                    </div>
                  ),
                  th: ({node, ...props}) => (
                    <th 
                      className="border px-4 py-2 text-left font-bold [&[align=center]]:text-center [&[align=right]]:text-right"
                      {...props}
                    />
                  ),
                  td: ({node, ...props}) => (
                    <td 
                      className="border px-4 py-2 text-left [&[align=center]]:text-center [&[align=right]]:text-right"
                      {...props}
                    />
                  ),
                }}
              >
                {content}
              </ReactMarkdown>
            </div>

            <div className="w-64 flex-none sticky top-24">
              <div className="space-y-6">
                <div className="mb-8">
                  <h3 className="font-medium mb-4">ON THIS PAGE</h3>
                  <nav className="space-y-2">
                    {navigation
                      .flatMap(section => section.pages)
                      .find(page => page.id === activePage)
                      ?.subsections.map(subsection => (
                        <button
                          key={subsection.id}
                          onClick={() => scrollToSection(subsection.id)}
                          className="block text-sm text-gray-600 dark:text-gray-300 hover:text-gray-900"
                        >
                          {subsection.title}
                        </button>
                      ))}
                  </nav>
                </div>

                <div className="space-y-6">
                  <h3 className="font-medium">MORE</h3>
                  <nav className="space-y-2">
                    <Button variant="ghost" className="w-full justify-start gap-2 h-8">
                      <Edit className="h-4 w-4" />
                      Edit this page
                    </Button>
                    <Button variant="ghost" className="w-full justify-start gap-2 h-8">
                      <MessageSquare className="h-4 w-4" />
                      Join our community
                    </Button>
                  </nav>

                  <div className="flex items-center gap-2">
                    <Button
                      variant="ghost"
                      size="icon"
                      className="h-8 w-8"
                      onClick={() => setIsDarkMode(!isDarkMode)}
                    >
                      <Sun className="h-4 w-4 dark:hidden" />
                      <Moon className="hidden h-4 w-4 dark:block" />
                    </Button>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </main>
      </div>
    </div>
  );
};

export default DocLayout;