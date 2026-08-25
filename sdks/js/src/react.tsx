import React, { createContext, useContext, useEffect, useState, useMemo } from 'react';
import { FlaguraClient, EvaluationContext, EvaluationResult, FlaguraClientOptions } from './index';

interface FlaguraContextType {
  client: FlaguraClient;
  defaultContext?: Partial<EvaluationContext>;
}

const FlaguraReactContext = createContext<FlaguraContextType | null>(null);

export interface FlaguraProviderProps extends FlaguraClientOptions {
  children: React.ReactNode;
  defaultContext?: Partial<EvaluationContext>;
}

export function FlaguraProvider({
  children,
  endpoint,
  apiKey,
  defaultEnvironment,
  timeoutMs,
  defaultContext,
}: FlaguraProviderProps) {
  const client = useMemo(
    () =>
      new FlaguraClient({
        endpoint,
        apiKey,
        defaultEnvironment,
        timeoutMs,
      }),
    [endpoint, apiKey, defaultEnvironment, timeoutMs]
  );

  return (
    <FlaguraReactContext.Provider value={{ client, defaultContext }}>
      {children}
    </FlaguraReactContext.Provider>
  );
}

export interface UseFeatureFlagResult<T = any> {
  isEnabled: boolean;
  variant: string;
  value: T;
  loading: boolean;
  error: Error | null;
  result: EvaluationResult<T> | null;
}

export function useFeatureFlag<T = any>(
  flagKey: string,
  userContext: EvaluationContext,
  fallbackValue?: T
): UseFeatureFlagResult<T> {
  const ctx = useContext(FlaguraReactContext);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<Error | null>(null);
  const [result, setResult] = useState<EvaluationResult<T> | null>(null);

  const mergedContext = useMemo(
    () => ({
      ...(ctx?.defaultContext || {}),
      ...userContext,
    }),
    [ctx?.defaultContext, userContext]
  );

  useEffect(() => {
    let isMounted = true;
    setLoading(true);
    setError(null);

    const client = ctx?.client || new FlaguraClient({ endpoint: window?.location?.origin || 'http://localhost:3000' });

    client
      .evaluate<T>(flagKey, mergedContext)
      .then((res) => {
        if (isMounted) {
          setResult(res);
          setLoading(false);
        }
      })
      .catch((err) => {
        if (isMounted) {
          setError(err instanceof Error ? err : new Error(String(err)));
          setLoading(false);
        }
      });

    return () => {
      isMounted = false;
    };
  }, [flagKey, JSON.stringify(mergedContext)]);

  return {
    isEnabled: result ? result.enabled : false,
    variant: result ? result.variant : 'off',
    value: result ? result.value : (fallbackValue as T),
    loading,
    error,
    result,
  };
}
