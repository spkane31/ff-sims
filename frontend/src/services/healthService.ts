import { apiClient } from './apiClient';

export interface HealthStatus {
  GitSHA: string;
  BuildTime: string;
  status?: 'healthy' | 'unhealthy';
  timestamp?: string;
}

export interface HealthServiceError {
  message: string;
  status?: number;
}

export const healthService = {
  /**
   * Check application health status
   */
  async checkHealth(): Promise<HealthStatus> {
    try {
      const response = await apiClient.get<HealthStatus>('/health');
      return {
        ...response,
        status: 'healthy',
        timestamp: new Date().toISOString(),
      };
    } catch (error) {
      console.error('Health check failed:', error);
      throw {
        message: error instanceof Error ? error.message : 'Health check failed',
        status: error instanceof Error && 'status' in error ? (error as Error & { status: number }).status : 500,
      } as HealthServiceError;
    }
  },

  /**
   * Get application version info
   */
  async getVersion(): Promise<{ version: string; buildTime: string }> {
    try {
      const health = await this.checkHealth();
      return {
        version: health.GitSHA || 'unknown',
        buildTime: health.BuildTime || 'unknown',
      };
    } catch (error) {
      throw error;
    }
  }
};