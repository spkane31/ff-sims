/**
 * Generates a random number from a normal distribution.
 *
 * @param {number} mean - The mean value of the distribution.
 * @param {number} std - The standard deviation of the distribution.
 * @returns {number} - A random number from the normal distribution.
 */
//
export const normalDistribution = (mean, std) => {
  let u = 0,
    v = 0;
  while (u === 0) u = Math.random();
  while (v === 0) v = Math.random();
  return (
    mean + std * Math.sqrt(-2.0 * Math.log(u)) * Math.cos(2.0 * Math.PI * v)
  );
};
