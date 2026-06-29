/** @type {import('next').NextConfig} */
const nextConfig = {
  // Disable static export to support dynamic routes
  // output: process.env.NODE_ENV === "production" ? "export" : undefined,
  distDir: ".next",
  /* other config options here */
};

module.exports = nextConfig;