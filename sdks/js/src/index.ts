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
}

export class FlaguraClient {
  private endpoint: string;
  private apiKey?: string;
  private defaultEnvironment: string;
  private timeoutMs: number;

  constructor(options: FlaguraClientOptions) {
    this.endpoint = options.endpoint.replace(/\/+$/, '');
    this.apiKey = options.apiKey;
    this.defaultEnvironment = options.defaultEnvironment || 'production';
    this.timeoutMs = options.timeoutMs || 5000;
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
        throw new Error(`Flagura evaluation failed with HTTP status ${response.status}`);
      }

      const data = await response.json();
      return (data.results || {}) as Record<string, EvaluationResult<T>>;
    } finally {
      clearTimeout(timeout);
    }
  }
}

export * from './openfeature';
