# syntax=docker/dockerfile:1
# context: repo root (client imports ../contracts/deployments). NEXT_PUBLIC_* are baked in at build time.
FROM node:22-bookworm-slim AS build
RUN npm i -g bun@1.3.13
WORKDIR /src
COPY contracts/deployments ./contracts/deployments
COPY client/package.json client/bun.lock ./client/
WORKDIR /src/client
RUN bun install --frozen-lockfile
COPY client ./
ARG NEXT_PUBLIC_NETWORK=testnet
ARG NEXT_PUBLIC_RPC_URL=https://rpc.testnet.near.org
ARG NEXT_PUBLIC_SITE_URL
ARG NEXT_PUBLIC_API_URL
ARG NEXT_PUBLIC_WS_URL
ARG NEXT_PUBLIC_MARKET_DATA_URL
ARG NEXT_PUBLIC_HISTORY_URL
RUN bun run build

# standalone output mirrors the turbopack root (repo root), so the server lives at client/server.js
FROM node:22-bookworm-slim
WORKDIR /app
ENV NODE_ENV=production HOSTNAME=0.0.0.0 PORT=3000
COPY --from=build --chown=node:node /src/client/.next/standalone ./
COPY --from=build --chown=node:node /src/client/.next/static ./client/.next/static
COPY --from=build --chown=node:node /src/client/public ./client/public
EXPOSE 3000
USER node
CMD ["node", "client/server.js"]
