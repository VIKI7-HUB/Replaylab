import React from 'react';
import { Drawer } from '../../components/Drawer';
import { Capture } from '../../api/replaylab';
import { StatusLabel } from '../../components/StatusLabel';
import { CodeBlock } from '../../components/CodeBlock';
import { formatDuration, formatIsoUtc, formatMoney, formatTokens } from '../../lib/format';

export interface CaptureDrawerProps {
  capture: Capture | null;
  isOpen: boolean;
  onClose: () => void;
  onNavigatePrev?: () => void;
  onNavigateNext?: () => void;
}

export const CaptureDrawer: React.FC<CaptureDrawerProps> = ({
  capture,
  isOpen,
  onClose,
  onNavigatePrev,
  onNavigateNext,
}) => {
  if (!capture) return null;

  // Format message text with redaction tokens highlighted
  const renderMessageContent = (content: string) => {
    if (!content) return <span>—</span>;

    const redactionRegex = /(\[REDACTED_EMAIL\]|\[REDACTED_PHONE\]|\[REDACTED_CARD\]|\[REDACTED_API_KEY\])/g;
    const parts = content.split(redactionRegex);

    return (
      <span className="whitespace-pre-wrap">
        {parts.map((part, i) => {
          if (part.match(redactionRegex)) {
            return (
              <span
                key={i}
                className="mono-ui text-[12px] bg-warning-bg text-warning px-1 rounded-sm mx-0.5 inline-block"
              >
                {part}
              </span>
            );
          }
          return part;
        })}
      </span>
    );
  };

  // Parse messages JSON
  let messages: Array<{ role: string; content: string }> = [];
  try {
    const parsed = JSON.parse(capture.messages_json);
    if (Array.isArray(parsed)) messages = parsed;
  } catch {
    messages = [];
  }

  // Check if response is JSON
  let isResponseJson = false;
  let parsedResponse = '';
  try {
    const parsed = JSON.parse(capture.response_json);
    if (parsed && typeof parsed === 'object') {
      isResponseJson = true;
      parsedResponse = JSON.stringify(parsed, null, 2);
    }
  } catch {
    isResponseJson = false;
  }

  return (
    <Drawer
      isOpen={isOpen}
      onClose={onClose}
      title={capture.id}
      onNavigatePrev={onNavigatePrev}
      onNavigateNext={onNavigateNext}
    >
      {/* 1. Summary grid: 2 columns */}
      <div>
        <div className="text-[13px] font-medium text-text-2 mb-3">Summary</div>
        <div className="grid grid-cols-2 gap-y-2.5 gap-x-4 bg-bg border border-border rounded-md p-4">
          <div>
            <div className="text-[12px] text-text-2">Request ID</div>
            <div className="mono-ui text-[13px] text-text truncate">{capture.request_id || capture.id}</div>
          </div>
          <div>
            <div className="text-[12px] text-text-2">Time</div>
            <div className="mono-ui text-[13px] text-text">{formatIsoUtc(capture.timestamp)}</div>
          </div>
          <div>
            <div className="text-[12px] text-text-2">Provider</div>
            <div className="text-[13px] text-text">{capture.provider || '—'}</div>
          </div>
          <div>
            <div className="text-[12px] text-text-2">Model</div>
            <div className="mono-ui text-[13px] text-text">{capture.model}</div>
          </div>
          <div>
            <div className="text-[12px] text-text-2">Virtual key</div>
            <div className="mono-ui text-[13px] text-text">{capture.virtual_key || '—'}</div>
          </div>
          <div>
            <div className="text-[12px] text-text-2">Tenant</div>
            <div className="text-[13px] text-text">{capture.tenant || '—'}</div>
          </div>
          <div>
            <div className="text-[12px] text-text-2">Tag</div>
            <div className="text-[13px] text-text">{capture.tag || '—'}</div>
          </div>
          <div>
            <div className="text-[12px] text-text-2">Status</div>
            <div className={`text-[13px] font-medium ${capture.status_code >= 400 ? 'text-danger' : 'text-text'}`}>
              {capture.status_code}
            </div>
          </div>
          <div>
            <div className="text-[12px] text-text-2">State</div>
            <div className="mt-0.5">
              <StatusLabel value={capture.completion_state} />
            </div>
          </div>
          <div>
            <div className="text-[12px] text-text-2">Prompt tokens</div>
            <div className="text-[13px] text-text tabular-nums">
              {capture.usage_estimated ? '≈ ' : ''}{formatTokens(capture.prompt_tokens)}
            </div>
          </div>
          <div>
            <div className="text-[12px] text-text-2">Completion tokens</div>
            <div className="text-[13px] text-text tabular-nums">
              {capture.usage_estimated ? '≈ ' : ''}{formatTokens(capture.completion_tokens)}
            </div>
          </div>
          <div>
            <div className="text-[12px] text-text-2">Latency</div>
            <div className="text-[13px] text-text tabular-nums">{formatDuration(capture.latency_ms)}</div>
          </div>
          <div>
            <div className="text-[12px] text-text-2">TTFT</div>
            <div className="text-[13px] text-text tabular-nums">
              {capture.time_to_first_token_ms ? formatDuration(capture.time_to_first_token_ms) : '—'}
            </div>
          </div>
          <div>
            <div className="text-[12px] text-text-2">Cost</div>
            <div className="text-[13px] text-text tabular-nums">
              {capture.usage_estimated ? '≈ ' : ''}{formatMoney(capture.cost_estimate)}
            </div>
          </div>
        </div>
      </div>

      {/* 2. Messages */}
      <div>
        <div className="text-[13px] font-medium text-text-2 mb-3">Messages</div>
        {capture.hash_only ? (
          <div className="text-[13px] text-text-2 bg-bg-muted p-3 rounded-md">
            Stored as hashes only. Content isn't available.
          </div>
        ) : messages.length > 0 ? (
          <div className="space-y-3">
            {messages.map((msg, i) => (
              <div key={i} className="bg-bg-subtle border border-border rounded-md p-3">
                <div className="text-[12px] font-medium text-text-2 capitalize mb-1">
                  {msg.role}
                </div>
                <div className="text-[14px] text-text leading-5">
                  {renderMessageContent(msg.content)}
                </div>
              </div>
            ))}
          </div>
        ) : (
          <div className="text-[13px] text-text-3">No messages</div>
        )}
      </div>

      {/* 3. Response */}
      <div>
        <div className="text-[13px] font-medium text-text-2 mb-3">Response</div>
        {capture.hash_only ? (
          <div className="text-[13px] text-text-2 bg-bg-muted p-3 rounded-md">
            Stored as hashes only. Content isn't available.
          </div>
        ) : isResponseJson ? (
          <CodeBlock code={parsedResponse} />
        ) : (
          <div className="bg-bg-subtle border border-border rounded-md p-3 text-[14px] text-text leading-5">
            {renderMessageContent(capture.response_json)}
          </div>
        )}
      </div>

      {/* 4. Parameters */}
      <div>
        <div className="text-[13px] font-medium text-text-2 mb-3">Parameters</div>
        <CodeBlock code={capture.params_json} />
      </div>

      {/* 5. Error */}
      {capture.error_message && (
        <div>
          <div className="text-[13px] font-medium text-danger mb-2">Error</div>
          <div className="text-[13px] text-danger bg-danger-bg p-3 rounded-md border border-danger">
            {capture.error_message}
          </div>
        </div>
      )}
    </Drawer>
  );
};

