import path from "node:path";
import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  reactCompiler: true,
  // self-contained server in .next/standalone for the Docker image (infra/client.Dockerfile)
  output: "standalone",
  // repo root, so lib/config can import contracts/deployments/*.json
  turbopack: { root: path.join(__dirname, "..") },
};

export default nextConfig;
