import React from 'react';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';

// Every colour below comes from a semantic token, so this flips with the
// theme instead of needing a `dark:` patch per element. Links and the
// blockquote rule use `text-primary-emphasis` / `border-primary-emphasis` —
// the token index.css already defines separately per theme (RED-INK 5.87:1
// on light, a lifted red in dark) — rather than a hand-picked hex repeated in
// both a light and a dark class.
const ProcessedText = ({ content, className = '' }) => {
  if (!content) return null;

  const components = {
    h1: ({ children }) => (
      <h1 className="mb-4 mt-8 text-2xl font-bold text-foreground">{children}</h1>
    ),
    h2: ({ children }) => (
      <h2 className="mb-3 mt-6 text-xl font-bold text-foreground">{children}</h2>
    ),
    h3: ({ children }) => (
      <h3 className="mb-2 mt-4 text-lg font-semibold text-foreground">{children}</h3>
    ),
    p: ({ children }) => (
      <p className="mt-4 leading-relaxed text-foreground">{children}</p>
    ),
    strong: ({ children }) => <strong className="font-semibold text-foreground">{children}</strong>,
    ul: ({ children }) => (
      <ul className="mt-4 list-none space-y-2">{children}</ul>
    ),
    ol: ({ children }) => (
      <ol className="mt-4 list-decimal space-y-2 pl-5 marker:text-muted-foreground">{children}</ol>
    ),
    li: ({ children, ordered }) =>
      ordered ? (
        <li className="text-foreground">{children}</li>
      ) : (
        <li className="flex items-center gap-2">
          <span className="h-1.5 w-1.5 shrink-0 rounded-full bg-muted-foreground" />
          <span className="text-foreground">{children}</span>
        </li>
      ),
    a: ({ href, children }) => (
      <a
        href={href}
        className="text-primary-emphasis underline underline-offset-4 transition-colors duration-200 hover:text-primary-emphasis/80"
        target="_blank"
        rel="noopener noreferrer"
      >
        {children}
      </a>
    ),
    code: ({ inline, children }) => (
      inline ?
        <code className="rounded bg-muted px-1.5 py-0.5 text-sm text-foreground">{children}</code> :
        <pre className="mb-4 mt-4 overflow-x-auto rounded-lg bg-muted p-4">
          <code className="text-sm text-foreground">{children}</code>
        </pre>
    ),
    blockquote: ({ children }) => (
      <blockquote className="my-4 border-l-4 border-primary-emphasis pl-4 italic text-muted-foreground">
        {children}
      </blockquote>
    ),
  };

  return (
    <div className={`max-w-none ${className}`}>
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
