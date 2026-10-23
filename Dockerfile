# syntax=docker/dockerfile:1

ARG GO_VERSION=1.26

# ---- dev: hot reload with air; source code is bind-mounted at /app ----
FROM golang:${GO_VERSION}-alpine AS dev
WORKDIR /app
RUN go install github.com/air-verse/air@v1.67.4
COPY go.mod go.sum ./
RUN go mod download
CMD ["air", "-c", ".air.toml"]

# ---- build: compile a static binary ----
FROM golang:${GO_VERSION}-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /bin/api ./cmd/api

# ---- runtime: minimal non-root image ----
FROM gcr.io/distroless/static-debian12:nonroot AS runtime
COPY --from=build /bin/api /api
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/api"]
