# syntax=docker/dockerfile:1
# build context: repo root (docker build -f executor/Dockerfile .)
FROM rust:1.95-slim-bookworm AS build
WORKDIR /src
COPY executor/Cargo.toml executor/Cargo.lock ./
RUN --mount=type=cache,target=/usr/local/cargo/registry \
    mkdir src && echo 'fn main() {}' > src/main.rs && touch src/lib.rs && cargo build --release --locked
COPY executor/src ./src
RUN --mount=type=cache,target=/usr/local/cargo/registry \
    touch src/main.rs src/lib.rs && cargo build --release --locked

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates && rm -rf /var/lib/apt/lists/*
COPY contracts/deployments /deployments
COPY --from=build /src/target/release/executor /usr/local/bin/executor
ENV EXECUTOR_ADDR=0.0.0.0:8083 DEPLOYMENTS_DIR=/deployments NEAR_NETWORK=testnet
EXPOSE 8083
USER nobody
ENTRYPOINT ["/usr/local/bin/executor"]
