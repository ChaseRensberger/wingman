FROM --platform=$BUILDPLATFORM oven/bun:1 AS console
WORKDIR /src/web
COPY web/ ./
RUN bun install --frozen-lockfile && bun run build:console

FROM --platform=$BUILDPLATFORM golang:1.25.3-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=console /src/web/apps/console/dist ./web/apps/console/dist
ARG VERSION=dev
ARG COMMIT=none
ARG DATE=unknown
ARG TARGETOS
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -tags webdist \
    -ldflags "-s -w -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.date=${DATE}" \
    -o /${TARGETARCH}/wingman ./cmd/wingman

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends \
    bash ca-certificates git ripgrep \
    && rm -rf /var/lib/apt/lists/* \
    && useradd --create-home --uid 10001 wingman \
    && mkdir /data && chown wingman:wingman /data
# Releases replace the build stage with a context containing GoReleaser binaries.
ARG TARGETARCH
COPY --from=build --chmod=755 /${TARGETARCH}/wingman /usr/local/bin/wingman
USER wingman
ENV HOME=/home/wingman
VOLUME /data
EXPOSE 2424
ENTRYPOINT ["wingman"]
CMD ["serve", "--host", "0.0.0.0", "--db", "/data/wingman.db"]
