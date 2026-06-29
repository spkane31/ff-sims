import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  // Disable static export to support dynamic routes
  // output: process.env.NODE_ENV === "production" ? "export" : undefined,
  distDir: ".next",
  /* other config options here */
};

export default nextConfig;
