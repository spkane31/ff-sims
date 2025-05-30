/** @type {import('next').NextConfig} */
const nextConfig = {
  // Only use export for production builds, not for development
  output: process.env.NODE_ENV === "production" ? "export" : undefined,
  distDir: process.env.NODE_ENV === "production" ? "out" : ".next",
  /* other config options here */
};

module.exports = nextConfig;