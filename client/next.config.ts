import path from "node:path";
import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  reactCompiler: true,
  // repo root, so lib/config can import contracts/deployments/*.json
  turbopack: { root: path.join(__dirname, "..") },
};

export default nextConfig;
