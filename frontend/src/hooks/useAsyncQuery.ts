import { useCallback, useEffect, useRef, useState } from "react";

export interface AsyncQueryResult<T> {
  data: T;
  isLoading: boolean;
  error: Error | null;
  refetch: () => Promise<void>;
}

interface UseAsyncQueryOptions<T> {
  initialData: T;
  enabled?: boolean;
  errorMessage: string;
}

/**
 * Runs a client-side query and owns its loading, error, and refresh lifecycle.
 *
 * A newer request always wins, so a slow response from an outdated query cannot
 * overwrite the result of a later refresh or parameter change.
 */
export function useAsyncQuery<T>(
  query: () => Promise<T>,
  { initialData, enabled = true, errorMessage }: UseAsyncQueryOptions<T>
): AsyncQueryResult<T> {
  const [data, setData] = useState<T>(initialData);
  const [isLoading, setIsLoading] = useState(enabled);
  const [error, setError] = useState<Error | null>(null);
  const isMounted = useRef(true);
  const latestRequest = useRef(0);

  useEffect(() => {
    isMounted.current = true;

    return () => {
      isMounted.current = false;
    };
  }, []);

  const refetch = useCallback(async () => {
    const request = ++latestRequest.current;

    setIsLoading(true);
    setError(null);

    try {
      const nextData = await query();
      if (isMounted.current && latestRequest.current === request) {
        setData(nextData);
      }
    } catch (cause) {
      if (isMounted.current && latestRequest.current === request) {
        setError(cause instanceof Error ? cause : new Error(errorMessage));
      }
    } finally {
      if (isMounted.current && latestRequest.current === request) {
        setIsLoading(false);
      }
    }
  }, [errorMessage, query]);

  useEffect(() => {
    if (!enabled) {
      latestRequest.current += 1;
      setIsLoading(false);
      return;
    }

    void refetch();
  }, [enabled, refetch]);

  return { data, isLoading, error, refetch };
}
