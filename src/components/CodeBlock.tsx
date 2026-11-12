import React from 'react';
import { CopyButton } from './CopyButton';

export interface CodeBlockProps {
  code: string;
  isJson?: boolean;
  className?: string;
}

export const CodeBlock: React.FC<CodeBlockProps> = ({ code, isJson = true, className = '' }) => {
  let formatted = code;
  let parsedJson: any = null;

  if (isJson) {
    try {
      parsedJson = typeof code === 'string' ? JSON.parse(code) : code;
      formatted = JSON.stringify(parsedJson, null, 2);
    } catch {
      // not valid json, leave as raw string
    }
  }

  // Syntax highlighting for JSON according to §12.1:
  // keys --text, strings --accent, numbers --warning, true/false/null --danger, punctuation --text-3
  const highlightJson = (jsonStr: string) => {
    const escaped = jsonStr
      .replace(/&/g, '&amp;')
      .replace(/</g, '&lt;')
      .replace(/>/g, '&gt;');

    return escaped.replace(
      /("(\\u[a-zA-Z0-9]{4}|\\[^u]|[^\\"])*"(\s*:)?|\b(true|false|null)\b|-?\d+(?:\.\d*)?(?:[eE][+-]?\d+)?|[{}[\],:])/g,
      (match) => {
        if (/^"/.test(match)) {
          if (/:$/.test(match)) {
            // Key
            return `<span style="color: var(--text);">${match}</span>`;
          } else {
            // String
            return `<span style="color: var(--accent);">${match}</span>`;
          }
        } else if (/true|false|null/.test(match)) {
          // Boolean / null
          return `<span style="color: var(--danger);">${match}</span>`;
        } else if (/^-?\d/.test(match)) {
          // Number
          return `<span style="color: var(--warning);">${match}</span>`;
        } else {
          // Punctuation
          return `<span style="color: var(--text-3);">${match}</span>`;
        }
      }
    );
  };

  return (
    <div className={`relative bg-bg-subtle border border-border rounded-md p-3 mono-ui text-[12.5px] leading-[18px] overflow-x-auto ${className}`}>
      <div className="absolute top-2 right-2 z-10">
        <CopyButton value={formatted} tooltipText="Copy code" />
      </div>
      <pre className="pr-8 m-0 font-mono whitespace-pre font-normal">
        {parsedJson ? (
          <code dangerouslySetInnerHTML={{ __html: highlightJson(formatted) }} />
        ) : (
          <code>{code}</code>
        )}
      </pre>
    </div>
  );
};

