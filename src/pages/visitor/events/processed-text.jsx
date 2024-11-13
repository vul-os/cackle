import React from 'react';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';

const ProcessedText = ({ content, className = "" }) => {
  if (!content) return null;

  const components = {
    // Custom heading styles
    h1: ({ children }) => (
      <h1 className="text-2xl font-bold text-white mt-8 mb-4">{children}</h1>
    ),
    h2: ({ children }) => (
      <h2 className="text-xl font-bold text-white mt-6 mb-3">{children}</h2>
    ),
    h3: ({ children }) => (
      <h3 className="text-lg font-semibold text-white mt-4 mb-2">{children}</h3>
    ),
    // Custom paragraph styles
    p: ({ children }) => (
      <p className="text-gray-200 mt-4 leading-relaxed">{children}</p>
    ),
    // Custom list styles
    ul: ({ children }) => (
      <ul className="mt-4 space-y-2">{children}</ul>
    ),
    li: ({ children }) => (
      <li className="flex items-center gap-2">
        <span className="w-1.5 h-1.5 bg-white/50 rounded-full flex-shrink-0" />
        <span className="text-gray-200">{children}</span>
      </li>
    ),
    // Custom link styles
    a: ({ href, children }) => (
      <a 
        href={href}
        className="text-[#880424] hover:text-[#a31838] underline transition-colors duration-200"
        target="_blank"
        rel="noopener noreferrer"
      >
        {children}
      </a>
    ),
    // Custom code block styles
    code: ({ inline, children }) => (
      inline ? 
        <code className="bg-black/20 px-1.5 py-0.5 rounded text-sm">{children}</code> :
        <pre className="bg-black/20 p-4 rounded-lg mt-4 mb-4 overflow-x-auto">
          <code className="text-sm">{children}</code>
        </pre>
    ),
    // Custom blockquote styles
    blockquote: ({ children }) => (
      <blockquote className="border-l-4 border-[#880424] pl-4 italic my-4 text-gray-300">
        {children}
      </blockquote>
    ),
  };

  return (
    <div className={`prose prose-invert max-w-none ${className}`}>
      <ReactMarkdown
        remarkPlugins={[remarkGfm]}
        components={components}
      >
        {content}
      </ReactMarkdown>
    </div>
  );
};

export default ProcessedText;