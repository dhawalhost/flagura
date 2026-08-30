import { FlaguraClient, EvaluationContext as FlaguraContext } from './index';

export interface OpenFeatureResolutionDetails<T> {
  value: T;
  variant?: string;
  reason?: 'STATIC' | 'DEFAULT' | 'TARGETING_MATCH' | 'SPLIT' | 'DISABLED' | 'ERROR' | string;
  errorCode?: string;
  errorMessage?: string;
}

export interface OpenFeatureEvaluationContext {
  targetingKey?: string;
  userId?: string;
  email?: string;
  country?: string;
  role?: string;
  tier?: string;
  environment?: string;
  [key: string]: any;
}

/**
 * Flagura OpenFeature Provider for JavaScript and TypeScript.
 * Compatible with OpenFeature Server & Web SDK standards.
 */
export class FlaguraOpenFeatureProvider {
  readonly metadata = {
    name: 'flagura-openfeature-provider',
  };

  private client: FlaguraClient;

  constructor(client: FlaguraClient) {
    this.client = client;
  }

  private mapContext(context?: OpenFeatureEvaluationContext): FlaguraContext {
    const ctx: FlaguraContext = {
      user_id: context?.targetingKey || context?.userId || context?.user_id || 'anonymous',
    };

    if (context) {
      if (context.email) ctx.email = context.email;
      if (context.country) ctx.country = context.country;
      if (context.role) ctx.role = context.role;
      if (context.tier) ctx.tier = context.tier;
      if (context.environment) ctx.environment = context.environment;

      // Extract custom attributes
      const custom: Record<string, any> = {};
      for (const [k, v] of Object.entries(context)) {
        if (!['targetingKey', 'userId', 'user_id', 'email', 'country', 'role', 'tier', 'environment'].includes(k)) {
          custom[k] = v;
        }
      }
      if (Object.keys(custom).length > 0) {
        ctx.custom = custom;
      }
    }

    return ctx;
  }

  private mapReason(reason: string): string {
    const r = (reason || '').toUpperCase();
    if (r.includes('TARGETING') || r.includes('RULE')) return 'TARGETING_MATCH';
    if (r.includes('PERCENTAGE') || r.includes('MULTIVARIATE') || r.includes('BUCKET')) return 'SPLIT';
    if (r.includes('KILL_SWITCH') || r.includes('ENV_DISABLED') || r.includes('DISABLED')) return 'DISABLED';
    if (r.includes('DEFAULT')) return 'DEFAULT';
    return 'STATIC';
  }

  async resolveBooleanEvaluation(
    flagKey: string,
    defaultValue: boolean,
    context?: OpenFeatureEvaluationContext
  ): Promise<OpenFeatureResolutionDetails<boolean>> {
    try {
      const evalCtx = this.mapContext(context);
      const res = await this.client.evaluate(flagKey, evalCtx);

      if (res.reason === 'FLAG_NOT_FOUND') {
        return {
          value: defaultValue,
          reason: 'ERROR',
          errorCode: 'FLAG_NOT_FOUND',
          errorMessage: `Flag '${flagKey}' not found`,
        };
      }

      if (!res.enabled) {
        return {
          value: defaultValue,
          variant: res.variant || 'off',
          reason: this.mapReason(res.reason),
        };
      }

      let val = defaultValue;
      if (typeof res.value === 'boolean') {
        val = res.value;
      } else if (res.value !== undefined && res.value !== null) {
        val = Boolean(res.value);
      } else {
        val = true;
      }

      return {
        value: val,
        variant: res.variant || 'treatment',
        reason: this.mapReason(res.reason),
      };
    } catch (err: any) {
      return {
        value: defaultValue,
        reason: 'ERROR',
        errorCode: 'GENERAL',
        errorMessage: err?.message || 'Flagura evaluation error',
      };
    }
  }

  async resolveStringEvaluation(
    flagKey: string,
    defaultValue: string,
    context?: OpenFeatureEvaluationContext
  ): Promise<OpenFeatureResolutionDetails<string>> {
    try {
      const evalCtx = this.mapContext(context);
      const res = await this.client.evaluate(flagKey, evalCtx);

      if (res.reason === 'FLAG_NOT_FOUND') {
        return {
          value: defaultValue,
          reason: 'ERROR',
          errorCode: 'FLAG_NOT_FOUND',
          errorMessage: `Flag '${flagKey}' not found`,
        };
      }

      if (!res.enabled) {
        return {
          value: defaultValue,
          variant: res.variant || 'off',
          reason: this.mapReason(res.reason),
        };
      }

      const val = typeof res.value === 'string' && res.value !== '' ? res.value : res.variant || defaultValue;

      return {
        value: val,
        variant: res.variant || 'treatment',
        reason: this.mapReason(res.reason),
      };
    } catch (err: any) {
      return {
        value: defaultValue,
        reason: 'ERROR',
        errorCode: 'GENERAL',
        errorMessage: err?.message || 'Flagura evaluation error',
      };
    }
  }

  async resolveNumberEvaluation(
    flagKey: string,
    defaultValue: number,
    context?: OpenFeatureEvaluationContext
  ): Promise<OpenFeatureResolutionDetails<number>> {
    try {
      const evalCtx = this.mapContext(context);
      const res = await this.client.evaluate(flagKey, evalCtx);

      if (res.reason === 'FLAG_NOT_FOUND') {
        return {
          value: defaultValue,
          reason: 'ERROR',
          errorCode: 'FLAG_NOT_FOUND',
          errorMessage: `Flag '${flagKey}' not found`,
        };
      }

      if (!res.enabled) {
        return {
          value: defaultValue,
          variant: res.variant || 'off',
          reason: this.mapReason(res.reason),
        };
      }

      const num = Number(res.value);
      const val = isNaN(num) ? defaultValue : num;

      return {
        value: val,
        variant: res.variant || 'treatment',
        reason: this.mapReason(res.reason),
      };
    } catch (err: any) {
      return {
        value: defaultValue,
        reason: 'ERROR',
        errorCode: 'GENERAL',
        errorMessage: err?.message || 'Flagura evaluation error',
      };
    }
  }

  async resolveObjectEvaluation<T = any>(
    flagKey: string,
    defaultValue: T,
    context?: OpenFeatureEvaluationContext
  ): Promise<OpenFeatureResolutionDetails<T>> {
    try {
      const evalCtx = this.mapContext(context);
      const res = await this.client.evaluate<T>(flagKey, evalCtx);

      if (res.reason === 'FLAG_NOT_FOUND') {
        return {
          value: defaultValue,
          reason: 'ERROR',
          errorCode: 'FLAG_NOT_FOUND',
          errorMessage: `Flag '${flagKey}' not found`,
        };
      }

      if (!res.enabled) {
        return {
          value: defaultValue,
          variant: res.variant || 'off',
          reason: this.mapReason(res.reason),
        };
      }

      const val = res.value !== undefined && res.value !== null ? res.value : defaultValue;

      return {
        value: val,
        variant: res.variant || 'treatment',
        reason: this.mapReason(res.reason),
      };
    } catch (err: any) {
      return {
        value: defaultValue,
        reason: 'ERROR',
        errorCode: 'GENERAL',
        errorMessage: err?.message || 'Flagura evaluation error',
      };
    }
  }
}
