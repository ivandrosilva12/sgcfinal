# syntax=docker/dockerfile:1

# ---- Etapa de compilação ----
FROM golang:1.25 AS build
WORKDIR /src

# Cache de dependências.
COPY go.mod go.sum ./
RUN go mod download

# Código e compilação estática (binário auto-suficiente para distroless).
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api

# ---- Imagem final (mínima, non-root) ----
FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /
COPY --from=build /out/api /api

EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/api"]
