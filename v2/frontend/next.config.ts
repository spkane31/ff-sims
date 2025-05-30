import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  // Only use export for production builds, not for development
  output: process.env.NODE_ENV === "production" ? "export" : undefined,
  distDir: process.env.NODE_ENV === "production" ? "out" : ".next",
  /* other config options here */
};

export default nextConfig;
