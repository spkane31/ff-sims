import { useState, useEffect } from 'react';
import { SimulationResult, SimulationMetrics, simulationService } from '../services/simulationService';

/**
 * Hook for accessing simulation results
 */
export function useSimulationResults() {
  const [results, setResults] = useState<SimulationResult[]>([]);
  const [isLoading, setIsLoading] = useState<boolean>(true);
  const [error, setError] = useState<Error | null>(null);

  const fetchResults = async () => {
    try {
      setIsLoading(true);
      setError(null);
      const data = await simulationService.getSimulationResults();
      setResults(data);
    } catch (err) {
      setError(err instanceof Error ? err : new Error('An error occurred while fetching simulation results'));
    } finally {
      setIsLoading(false);
    }
  };

  useEffect(() => {
    fetchResults();
  }, []);

  return { results, isLoading, error, refetch: fetchResults };
}

/**
 * Hook for accessing simulation metrics
 */
export function useSimulationMetrics() {
  const [metrics, setMetrics] = useState<SimulationMetrics | null>(null);
  const [isLoading, setIsLoading] = useState<boolean>(true);
  const [error, setError] = useState<Error | null>(null);

  const fetchMetrics = async () => {
    try {
      setIsLoading(true);
      setError(null);
      const data = await simulationService.getSimulationMetrics();
      setMetrics(data);
    } catch (err) {
      setError(err instanceof Error ? err : new Error('An error occurred while fetching simulation metrics'));
    } finally {
      setIsLoading(false);
    }
  };

  useEffect(() => {
    fetchMetrics();
  }, []);

  return { metrics, isLoading, error, refetch: fetchMetrics };
}

/**
 * Hook for running a new simulation
 */
export function useRunSimulation() {
  const [results, setResults] = useState<SimulationResult[]>([]);
  const [isLoading, setIsLoading] = useState<boolean>(false);
  const [error, setError] = useState<Error | null>(null);

  const runSimulation = async (params: { iterations?: number; seed?: number } = {}) => {
    try {
      setIsLoading(true);
      setError(null);
      const data = await simulationService.runSimulation(params);
      setResults(data);
      return data;
    } catch (err) {
      const errorMessage = err instanceof Error ? err.message : 'An error occurred while running simulation';
      setError(new Error(errorMessage));
      throw err;
    } finally {
      setIsLoading(false);
    }
  };

  return { results, isLoading, error, runSimulation };
}