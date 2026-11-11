# syntax=docker/dockerfile:1
# shared by every Go service; context is the repo root, pick one with --build-arg SERVICE=api
FROM golang:1.25-alpine AS build
ARG SERVICE
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY internal ./internal
COPY services ./services
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/app ./services/${SERVICE}/cmd/${SERVICE}

FROM alpine:3.22
RUN apk add --no-cache ca-certificates
COPY contracts/deployments /deployments
COPY --from=build /out/app /usr/local/bin/app
ENV DEPLOYMENTS_DIR=/deployments
USER nobody
ENTRYPOINT ["/usr/local/bin/app"]
