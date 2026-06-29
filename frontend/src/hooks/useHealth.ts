import { useState, useEffect, useCallback } from 'react';
import { healthService, HealthStatus, HealthServiceError } from '../services/healthService';

interface UseHealthReturn {
  healthStatus: HealthStatus | null;
  isLoading: boolean;
  error: HealthServiceError | null;
  isHealthy: boolean;
  checkHealth: () => Promise<void>;
  lastChecked: string | null;
}

export function useHealth(autoCheck: boolean = false, interval: number = 30000): UseHealthReturn {
  const [healthStatus, setHealthStatus] = useState<HealthStatus | null>(null);
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<HealthServiceError | null>(null);
  const [lastChecked, setLastChecked] = useState<string | null>(null);

  const checkHealth = useCallback(async () => {
    try {
      setIsLoading(true);
      setError(null);
      const status = await healthService.checkHealth();
      setHealthStatus(status);
      setLastChecked(new Date().toISOString());
    } catch (err) {
      const healthError = err as HealthServiceError;
      setError(healthError);
      setHealthStatus({
        GitSHA: 'unknown',
        BuildTime: 'unknown',
        status: 'unhealthy',
        timestamp: new Date().toISOString(),
      });
    } finally {
      setIsLoading(false);
    }
  }, []);

  useEffect(() => {
    if (autoCheck) {
      // Initial check
      checkHealth();

      // Set up interval if specified
      if (interval > 0) {
        const intervalId = setInterval(checkHealth, interval);
        return () => clearInterval(intervalId);
      }
    }
  }, [autoCheck, interval, checkHealth]);

  const isHealthy = healthStatus?.status === 'healthy' && !error;

  return {
    healthStatus,
    isLoading,
    error,
    isHealthy,
    checkHealth,
    lastChecked,
  };
}