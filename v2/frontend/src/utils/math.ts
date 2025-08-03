/**
 * Generate a random number from a normal distribution using Box-Muller transform
 * @param mean - The mean of the distribution
 * @param stdDev - The standard deviation of the distribution
 * @returns A random number from the normal distribution
 */
export function normalDistribution(mean: number, stdDev: number): number {
  let u = 0, v = 0;
  while(u === 0) u = Math.random(); // Converting [0,1) to (0,1)
  while(v === 0) v = Math.random();
  
  const z = Math.sqrt(-2.0 * Math.log(u)) * Math.cos(2.0 * Math.PI * v);
  return z * stdDev + mean;
}

/**
 * Calculate the variance of an array of numbers
 * @param values - Array of numbers
 * @returns The variance
 */
export function variance(values: number[]): number {
  if (values.length === 0) return 0;
  
  const mean = values.reduce((sum, val) => sum + val, 0) / values.length;
  const squaredDiffs = values.map(val => Math.pow(val - mean, 2));
  return squaredDiffs.reduce((sum, val) => sum + val, 0) / values.length;
}

/**
 * Calculate the standard deviation of an array of numbers
 * @param values - Array of numbers
 * @returns The standard deviation
 */
export function standardDeviation(values: number[]): number {
  return Math.sqrt(variance(values));
}