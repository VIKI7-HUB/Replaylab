import React, { useState, useEffect } from 'react';
import { PageHeader } from '../../app/PageHeader';
import { Button } from '../../components/Button';
import { Input } from '../../components/Input';
import { Field } from '../../components/Field';
import { Switch } from '../../components/Switch';
import { Panel } from '../../components/Panel';
import { ErrorState, Skeleton } from '../../components/States';
import { ReplayLabConfig, getConfig, updateConfig } from '../../api/replaylab';
import { useToast } from '../../components/Toast';
import { errorMessage } from '../../api/error';

export const SettingsPage: React.FC = () => {
  const { toast } = useToast();
  const [initialConfig, setInitialConfig] = useState<ReplayLabConfig | null>(null);
  const [config, setConfig] = useState<ReplayLabConfig | null>(null);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // Tag input temp strings
  const [optInInput, setOptInInput] = useState('');
  const [optOutInput, setOptOutInput] = useState('');

  const fetchSettings = async () => {
    setLoading(true);
    setError(null);
    try {
      const cfg = await getConfig();
      setInitialConfig(cfg);
      setConfig(cfg);
    } catch (err) {
      setError(errorMessage(err, 'Failed to load settings'));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchSettings();
  }, []);

  if (loading) {
    return (
      <div className="space-y-6">
        <Skeleton height="h-8" width="w-48" />
        <Skeleton height="h-4" width="w-96" />
        <Skeleton height="h-64" />
      </div>
    );
  }

  if (error || !config) {
    return (
      <ErrorState
        title="Couldn't load settings"
        message={error || 'Failed to load'}
        onRetry={fetchSettings}
      />
    );
  }

  // Detect unsaved changes
  const hasChanges = initialConfig
    ? (
        config.enabled !== initialConfig.enabled ||
        config.sampling_rate !== initialConfig.sampling_rate ||
        JSON.stringify(config.opt_in_keys) !== JSON.stringify(initialConfig.opt_in_keys) ||
        JSON.stringify(config.opt_out_keys) !== JSON.stringify(initialConfig.opt_out_keys) ||
        config.redact_pii !== initialConfig.redact_pii ||
        config.store_hashes_only !== initialConfig.store_hashes_only ||
        config.max_stream_capture_bytes !== initialConfig.max_stream_capture_bytes ||
        config.retention_days !== initialConfig.retention_days ||
        config.default_budget_cap !== initialConfig.default_budget_cap
      )
    : false;

  const handleDiscard = () => {
    setConfig(initialConfig);
  };

  const handleSave = async () => {
    if (!config || !initialConfig) return;
    setSaving(true);

    // Compute diff
    const diff: Partial<ReplayLabConfig> = {};
    if (config.enabled !== initialConfig.enabled) diff.enabled = config.enabled;
    if (config.sampling_rate !== initialConfig.sampling_rate) diff.sampling_rate = config.sampling_rate;
    if (JSON.stringify(config.opt_in_keys) !== JSON.stringify(initialConfig.opt_in_keys))
      diff.opt_in_keys = config.opt_in_keys;
    if (JSON.stringify(config.opt_out_keys) !== JSON.stringify(initialConfig.opt_out_keys))
      diff.opt_out_keys = config.opt_out_keys;
    if (config.redact_pii !== initialConfig.redact_pii) diff.redact_pii = config.redact_pii;
    if (config.store_hashes_only !== initialConfig.store_hashes_only)
      diff.store_hashes_only = config.store_hashes_only;
    if (config.max_stream_capture_bytes !== initialConfig.max_stream_capture_bytes)
      diff.max_stream_capture_bytes = config.max_stream_capture_bytes;
    if (config.retention_days !== initialConfig.retention_days)
      diff.retention_days = config.retention_days;
    if (config.default_budget_cap !== initialConfig.default_budget_cap)
      diff.default_budget_cap = config.default_budget_cap;

    try {
      const updated = await updateConfig(diff);
      setInitialConfig(updated);
      setConfig(updated);
      toast('Settings saved', 'success');
    } catch (err) {
      toast(errorMessage(err, 'Failed to save settings'), 'danger');
    } finally {
      setSaving(false);
    }
  };

  const addTag = (type: 'opt_in' | 'opt_out', value: string) => {
    const trimmed = value.trim();
    if (!trimmed) return;
    if (type === 'opt_in') {
      if (!config.opt_in_keys.includes(trimmed)) {
        setConfig({ ...config, opt_in_keys: [...config.opt_in_keys, trimmed] });
      }
      setOptInInput('');
    } else {
      if (!config.opt_out_keys.includes(trimmed)) {
        setConfig({ ...config, opt_out_keys: [...config.opt_out_keys, trimmed] });
      }
      setOptOutInput('');
    }
  };

  const removeTag = (type: 'opt_in' | 'opt_out', tag: string) => {
    if (type === 'opt_in') {
      setConfig({
        ...config,
        opt_in_keys: config.opt_in_keys.filter((t) => t !== tag),
      });
    } else {
      setConfig({
        ...config,
        opt_out_keys: config.opt_out_keys.filter((t) => t !== tag),
      });
    }
  };

  return (
    <div className="pb-20">
      <PageHeader
        title="Settings"
        description="Capture and replay settings. Changes apply immediately."
      />

      <div className="space-y-6 max-w-[640px]">
        {/* Panel 1: Capture */}
        <Panel title="Capture">
          <div className="space-y-4">
            <Switch
              id="cfg-enabled"
              checked={config.enabled}
              onCheckedChange={(checked) => setConfig({ ...config, enabled: checked })}
              label="Capture traffic"
            />

            <Field label="Sampling rate" helperText="1 = every request">
              <Input
                type="number"
                step="0.05"
                min="0"
                max="1"
                value={config.sampling_rate}
                onChange={(e) =>
                  setConfig({ ...config, sampling_rate: parseFloat(e.target.value) || 0 })
                }
              />
            </Field>

            {/* Opt-in keys */}
            <Field label="Opt-in keys" helperText="Press Enter to add virtual key chips.">
              <div className="space-y-2">
                <div className="flex flex-wrap gap-1.5 min-h-[28px] p-1.5 border border-border-strong rounded bg-bg">
                  {config.opt_in_keys.map((k) => (
                    <span
                      key={k}
                      className="mono-ui text-[12px] bg-bg-muted text-text px-1.5 py-0.5 rounded-sm inline-flex items-center gap-1"
                    >
                      {k}
                      <button
                        type="button"
                        onClick={() => removeTag('opt_in', k)}
                        className="text-text-3 hover:text-text cursor-pointer"
                      >
                        ×
                      </button>
                    </span>
                  ))}
                  <input
                    type="text"
                    value={optInInput}
                    onChange={(e) => setOptInInput(e.target.value)}
                    onKeyDown={(e) => {
                      if (e.key === 'Enter') {
                        e.preventDefault();
                        addTag('opt_in', optInInput);
                      }
                    }}
                    placeholder={config.opt_in_keys.length === 0 ? 'Type key and press Enter' : ''}
                    className="flex-1 min-w-[120px] bg-transparent text-[13px] text-text outline-none"
                  />
                </div>
              </div>
            </Field>

            {/* Opt-out keys */}
            <Field label="Opt-out keys" helperText="Press Enter to add virtual key chips.">
              <div className="space-y-2">
                <div className="flex flex-wrap gap-1.5 min-h-[28px] p-1.5 border border-border-strong rounded bg-bg">
                  {config.opt_out_keys.map((k) => (
                    <span
                      key={k}
                      className="mono-ui text-[12px] bg-bg-muted text-text px-1.5 py-0.5 rounded-sm inline-flex items-center gap-1"
                    >
                      {k}
                      <button
                        type="button"
                        onClick={() => removeTag('opt_out', k)}
                        className="text-text-3 hover:text-text cursor-pointer"
                      >
                        ×
                      </button>
                    </span>
                  ))}
                  <input
                    type="text"
                    value={optOutInput}
                    onChange={(e) => setOptOutInput(e.target.value)}
                    onKeyDown={(e) => {
                      if (e.key === 'Enter') {
                        e.preventDefault();
                        addTag('opt_out', optOutInput);
                      }
                    }}
                    placeholder={config.opt_out_keys.length === 0 ? 'Type key and press Enter' : ''}
                    className="flex-1 min-w-[120px] bg-transparent text-[13px] text-text outline-none"
                  />
                </div>
              </div>
            </Field>

            <Switch
              id="cfg-pii"
              checked={config.redact_pii}
              onCheckedChange={(checked) => setConfig({ ...config, redact_pii: checked })}
              label="Redact PII in captures"
            />

            <div>
              <Switch
                id="cfg-hashes"
                checked={config.store_hashes_only}
                onCheckedChange={(checked) =>
                  setConfig({ ...config, store_hashes_only: checked })
                }
                label="Store hashes only"
              />
              <p className="text-[12px] text-text-3 mt-1 ml-7">
                Hashed captures can't be replayed.
              </p>
            </div>

            <Field label="Max stream capture bytes">
              <Input
                type="number"
                value={config.max_stream_capture_bytes}
                onChange={(e) =>
                  setConfig({
                    ...config,
                    max_stream_capture_bytes: parseInt(e.target.value, 10) || 65536,
                  })
                }
              />
            </Field>
          </div>
        </Panel>

        {/* Panel 2: Retention and budget */}
        <Panel title="Retention and budget">
          <div className="space-y-4">
            <Field label="Retention days" helperText="0 keeps captures forever">
              <Input
                type="number"
                value={config.retention_days}
                onChange={(e) =>
                  setConfig({ ...config, retention_days: parseInt(e.target.value, 10) || 0 })
                }
              />
            </Field>

            <Field label="Default budget cap">
              <Input
                type="number"
                step="0.01"
                unit="$"
                value={config.default_budget_cap}
                onChange={(e) =>
                  setConfig({
                    ...config,
                    default_budget_cap: parseFloat(e.target.value) || 10,
                  })
                }
              />
            </Field>
          </div>
        </Panel>

        {/* Read-only panel: Startup settings */}
        <Panel title="Startup settings">
          <p className="text-[12px] text-text-3 mb-4">
            Set when the service starts. Edit the service config to change them.
          </p>
          <div className="grid grid-cols-2 gap-y-3 gap-x-4">
            <div>
              <div className="text-[12px] text-text-2">Database path</div>
              <div className="mono-ui text-[12.5px] text-text truncate">
                {config.db_path || '—'}
              </div>
            </div>
            <div>
              <div className="text-[12px] text-text-2">Gateway URL</div>
              <div className="mono-ui text-[12.5px] text-text truncate">
                {config.gateway_url || '—'}
              </div>
            </div>
            <div>
              <div className="text-[12px] text-text-2">Max queue size</div>
              <div className="mono-ui text-[12.5px] text-text">{config.max_queue_size}</div>
            </div>
            <div>
              <div className="text-[12px] text-text-2">Worker count</div>
              <div className="mono-ui text-[12.5px] text-text">{config.worker_count}</div>
            </div>
            <div>
              <div className="text-[12px] text-text-2">API listen address</div>
              <div className="mono-ui text-[12.5px] text-text">
                {config.api_listen_addr || '—'}
              </div>
            </div>
          </div>
        </Panel>
      </div>

      {/* Unsaved changes bar (fixed at bottom 52px) */}
      {hasChanges && (
        <div className="fixed bottom-0 left-[232px] right-0 h-[52px] bg-bg border-t border-border px-6 flex items-center justify-between z-20">
          <span className="text-[13px] text-text font-medium">You have unsaved changes</span>
          <div className="flex items-center gap-2">
            <Button size="sm" variant="ghost" onClick={handleDiscard}>
              Discard
            </Button>
            <Button size="sm" variant="primary" loading={saving} onClick={handleSave}>
              Save changes
            </Button>
          </div>
        </div>
      )}
    </div>
  );
};

