# Multi-stage build: compile a static binary, then run on a minimal image.

FROM golang:1.26 AS build
WORKDIR /src

# Cache dependencies.
COPY go.mod go.sum ./
RUN go mod download

# Build.
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/server ./cmd/server

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /
COPY --from=build /out/server /server
COPY --from=build /src/config.yaml /config.yaml

# App Platform provides PORT; default to 8080 for local runs.
ENV PORT=8080
# Tuning parameters are read from this file (see config.yaml).
ENV CONFIG_PATH=/config.yaml
EXPOSE 8080

USER nonroot:nonroot
ENTRYPOINT ["/server"]
