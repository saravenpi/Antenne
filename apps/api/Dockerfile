# --- build ---
FROM golang:1.24-bookworm AS build
WORKDIR /src
COPY apps/api/go.mod apps/api/go.sum ./
RUN go mod download
COPY apps/api/ ./
RUN CGO_ENABLED=0 go build -o /out/poste ./cmd/poste

# --- runtime (needs ffmpeg for decode/encode) ---
FROM debian:bookworm-slim
RUN apt-get update \
 && apt-get install -y --no-install-recommends ffmpeg ca-certificates \
 && rm -rf /var/lib/apt/lists/*
WORKDIR /app
COPY --from=build /out/poste /app/poste
ENV PORT=4000 \
    STORAGE_DIR=/data/tracks \
    STREAM_DIR=/data/stream
EXPOSE 4000
VOLUME ["/data"]
CMD ["/app/poste"]
