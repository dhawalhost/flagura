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
  endpoint: string;
  apiKey?: string;
  defaultEnvironment?: string;
  timeoutMs?: number;
  enableStreaming?: boolean;
}

export class FlaguraClient {
  private endpoint: string;
  private apiKey?: string;
  private defaultEnvironment: string;
  private timeoutMs: number;
  private localFlags: Map<string, any> = new Map();
  private listeners: Array<(flags: Map<string, any>) => void> = [];
  private abortController: AbortController | null = null;

  constructor(options: FlaguraClientOptions) {
    this.endpoint = options.endpoint.replace(/\/+$/, '');
    this.apiKey = options.apiKey;
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
              const flags = JSON.parse(dataStr);
              if (Array.isArray(flags)) {
                this.localFlags.clear();
                for (const f of flags) {
                  this.localFlags.set(f.key, f);
                }
                for (const listener of this.listeners) {
                  listener(new Map(this.localFlags));
                }
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

    await fetch(`${this.endpoint}/api/v1/events`, {
      method: 'POST',
      headers,
      body: JSON.stringify({
        event: {
          flag_key: flagKey,
          variant,
          metric_name: metricName,
          value,
          user_id: userId,
          environment: this.defaultEnvironment,
          timestamp: new Date().toISOString(),
        },
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
