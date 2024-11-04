// components/ProcessedText.jsx
const ProcessedText = ({ content, className = "" }) => {
    const processContent = (text) => {
      if (!text) return "";
  
      // Process the text step by step
      let processed = text
        // Replace section headers (##) with styled headers
        .replace(/##\s?(.*?)\\n/g, '<h2 class="text-xl font-bold text-white mt-6 mb-3">$1</h2>')
        // Replace subsection headers (#) with styled headers
        .replace(/#\s?(.*?)\\n/g, '<h3 class="text-lg font-semibold text-white mt-4 mb-2">$1</h3>')
        // Handle bullet points with custom styling
        .replace(/\\n-\s?(.*?)(?=\\n|$)/g, '<li class="flex items-center gap-2"><span class="w-1.5 h-1.5 bg-white/50 rounded-full"></span>$1</li>')
        // Convert checkmarks to styled spans
        .replace(/✅/g, '<span class="text-green-500">✓</span>')
        // Handle line breaks
        .split('\\n\\n').join('</p><p class="mt-4">')
        .split('\\n').join('<br>')
        // Wrap in paragraph if not already wrapped
        .replace(/^(?!<[hp])/g, '<p>');
  
      return processed;
    };
  
    return (
      <div 
        className={`text-gray-200 leading-relaxed ${className}`}
        dangerouslySetInnerHTML={{ __html: processContent(content) }}
      />
    );
  };
  
  export default ProcessedText;