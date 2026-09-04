export interface EvaluationContext {
  user_id: string;
  email?: string;
  country?: string;
  role?: string;
  tier?: string;
  environment?: 'production' | 'staging' | 'development' | string;
  [key: string]: any;
}

export interface EvaluationResult<T = any> {
  flag_key: string;
  enabled: boolean;
  variant: string;
  value: T;
  reason: string;
  bucket?: number;
  latency_ns?: number;
  latency_us?: number;
}

export interface FlaguraClientOptions {
  endpoint?: string;
  apiKey?: string;
  projectId?: string;
  defaultEnvironment?: string;
  timeoutMs?: number;
  enableStreaming?: boolean;
}

export class FlaguraClient {
  private endpoint: string;
  private apiKey?: string;
  private projectId?: string;
  private defaultEnvironment: string;
  private timeoutMs: number;
  private localFlags: Map<string, any> = new Map();
  private listeners: Array<(flags: Map<string, any>) => void> = [];
  private abortController: AbortController | null = null;

  constructor(options: FlaguraClientOptions = {}) {
    const nodeEnv = typeof globalThis !== 'undefined' ? (globalThis as any).process?.env?.FLAGURA_ENDPOINT : undefined;
    const rawEndpoint = options.endpoint || nodeEnv || 'https://flagura.dev';
    let ep = rawEndpoint.trim();
    if (!ep.startsWith('http://') && !ep.startsWith('https://')) {
      ep = (ep.startsWith('localhost') || ep.startsWith('127.0.0.1')) ? `http://${ep}` : `https://${ep}`;
    }
    this.endpoint = ep.replace(/\/+$/, '');
    this.apiKey = options.apiKey;
    this.projectId = options.projectId;
    this.defaultEnvironment = options.defaultEnvironment || 'production';
    this.timeoutMs = options.timeoutMs || 5000;

    if (options.enableStreaming) {
      this.startSSEStream();
    }
  }

  /**
   * Registers a listener callback invoked whenever feature flags update in real time.
   */
  onUpdate(callback: (flags: Map<string, any>) => void): () => void {
    this.listeners.push(callback);
    return () => {
      this.listeners = this.listeners.filter((l) => l !== callback);
    };
  }

  /**
   * Starts a persistent real-time Server-Sent Events stream from the Flagura control plane.
   */
  private async startSSEStream(): Promise<void> {
    if (typeof fetch === 'undefined') return;

    this.abortController = new AbortController();
    const url = `${this.endpoint}/api/v1/flags/stream`;
    const headers: Record<string, string> = { Accept: 'text/event-stream' };
    if (this.apiKey) {
      headers['Authorization'] = `Bearer ${this.apiKey}`;
    }
    if (this.projectId) {
      headers['X-Project-ID'] = this.projectId;
    }

    try {
      const response = await fetch(url, {
        headers,
        signal: this.abortController.signal,
      });

      if (!response.ok || !response.body) return;

      const reader = response.body.getReader();
      const decoder = new TextDecoder();
      let buffer = '';

      while (true) {
        const { value, done } = await reader.read();
        if (done) break;

        buffer += decoder.decode(value, { stream: true });
        const lines = buffer.split('\n');
        buffer = lines.pop() || '';

        for (let i = 0; i < lines.length; i++) {
          const line = lines[i].trim();
          if (line.startsWith('data:')) {
            try {
              const dataStr = line.slice(5).trim();
              if (dataStr === 'ping' || !dataStr) continue;
              const payload = JSON.parse(dataStr);
              if (Array.isArray(payload)) {
                this.localFlags.clear();
                for (const f of payload) {
                  this.localFlags.set(f.key, f);
                }
              } else if (payload.flags) {
                this.localFlags.clear();
                if (Array.isArray(payload.flags)) {
                  for (const f of payload.flags) this.localFlags.set(f.key, f);
                } else if (typeof payload.flags === 'object') {
                  for (const [k, v] of Object.entries(payload.flags)) this.localFlags.set(k, v);
                }
              }
              for (const listener of this.listeners) {
                listener(new Map(this.localFlags));
              }
            } catch {}
          }
        }
      }
    } catch {
      // Reconnect with backoff if not aborted
      if (this.abortController && !this.abortController.signal.aborted) {
        setTimeout(() => this.startSSEStream(), 3000);
      }
    }
  }

  async evaluate<T = any>(flagKey: string, context: EvaluationContext): Promise<EvaluationResult<T>> {
    const results = await this.evaluateBatch<T>([flagKey], context);
    const res = results[flagKey];
    if (!res) {
      return {
        flag_key: flagKey,
        enabled: false,
        variant: 'off',
        value: false as any,
        reason: 'FLAG_NOT_FOUND',
      };
    }
    return res;
  }

  async isEnabled(flagKey: string, context: EvaluationContext): Promise<boolean> {
    try {
      const res = await this.evaluate(flagKey, context);
      return res.enabled;
    } catch {
      return false;
    }
  }

  async getVariant(flagKey: string, context: EvaluationContext, fallback: string = 'off'): Promise<string> {
    try {
      const res = await this.evaluate(flagKey, context);
      return res.variant || fallback;
    } catch {
      return fallback;
    }
  }

  async evaluateBatch<T = any>(flagKeys: string[], context: EvaluationContext): Promise<Record<string, EvaluationResult<T>>> {
    const ctx = {
      environment: this.defaultEnvironment,
      ...context,
    };

    const headers: Record<string, string> = {
      'Content-Type': 'application/json',
    };
    if (this.apiKey) {
      headers['Authorization'] = `Bearer ${this.apiKey}`;
    }
    if (this.projectId) {
      headers['X-Project-ID'] = this.projectId;
    }

    const controller = new AbortController();
    const timeout = setTimeout(() => controller.abort(), this.timeoutMs);

    try {
      const response = await fetch(`${this.endpoint}/api/v1/evaluate`, {
        method: 'POST',
        headers,
        body: JSON.stringify({
          flags: flagKeys,
          context: ctx,
        }),
        signal: controller.signal,
      });

      if (!response.ok) {
        throw new Error(`Flagura evaluation failed with status ${response.status}`);
      }

      const data = await response.json();
      return data.results || {};
    } finally {
      clearTimeout(timeout);
    }
  }

  /**
   * Tracks an experiment conversion event.
   */
  async track(flagKey: string, variant: string, metricName: string, value: number = 1.0, userId: string = ''): Promise<void> {
    const headers: Record<string, string> = {
      'Content-Type': 'application/json',
    };
    if (this.apiKey) {
      headers['Authorization'] = `Bearer ${this.apiKey}`;
    }
    if (this.projectId) {
      headers['X-Project-ID'] = this.projectId;
    }

    await fetch(`${this.endpoint}/api/v1/telemetry/events`, {
      method: 'POST',
      headers,
      body: JSON.stringify({
        events: [
          {
            flag_key: flagKey,
            project_id: this.projectId,
            variant,
            metric_name: metricName,
            value,
            user_id: userId,
            environment: this.defaultEnvironment,
            timestamp: new Date().toISOString(),
          }
        ],
      }),
    });
  }

  /**
   * Closes active streaming connections.
   */
  close(): void {
    if (this.abortController) {
      this.abortController.abort();
      this.abortController = null;
    }
  }
}

export { FlaguraOpenFeatureProvider } from './openfeature';
