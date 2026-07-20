import React from 'react';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';

const ProcessedText = ({ content, className = "" }) => {
  if (!content) return null;

  const components = {
    h1: ({ children }) => (
      <h1 className="text-2xl font-bold text-slate-900 dark:text-white mt-8 mb-4">{children}</h1>
    ),
    h2: ({ children }) => (
      <h2 className="text-xl font-bold text-slate-800 dark:text-white mt-6 mb-3">{children}</h2>
    ),
    h3: ({ children }) => (
      <h3 className="text-lg font-semibold text-slate-800 dark:text-white mt-4 mb-2">{children}</h3>
    ),
    p: ({ children }) => (
      <p className="text-slate-700 dark:text-gray-200 mt-4 leading-relaxed">{children}</p>
    ),
    ul: ({ children }) => (
      <ul className="mt-4 space-y-2">{children}</ul>
    ),
    li: ({ children }) => (
      <li className="flex items-center gap-2">
        <span className="w-1.5 h-1.5 bg-slate-400 dark:bg-white/50 rounded-full flex-shrink-0" />
        <span className="text-slate-700 dark:text-gray-200">{children}</span>
      </li>
    ),
    a: ({ href, children }) => (
      <a 
        href={href}
        className="text-red-600 dark:text-[#880424] hover:text-red-700 dark:hover:text-[#a31838] underline transition-colors duration-200"
        target="_blank"
        rel="noopener noreferrer"
      >
        {children}
      </a>
    ),
    code: ({ inline, children }) => (
      inline ? 
        <code className="bg-slate-100 dark:bg-black/20 px-1.5 py-0.5 rounded text-sm">{children}</code> :
        <pre className="bg-slate-100 dark:bg-black/20 p-4 rounded-lg mt-4 mb-4 overflow-x-auto">
          <code className="text-sm">{children}</code>
        </pre>
    ),
    blockquote: ({ children }) => (
      <blockquote className="border-l-4 border-red-600 dark:border-[#880424] pl-4 italic my-4 text-slate-600 dark:text-gray-300">
        {children}
      </blockquote>
    ),
  };

  return (
    <div className={`prose prose-slate dark:prose-invert max-w-none ${className}`}>
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