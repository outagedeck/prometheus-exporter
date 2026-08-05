# syntax=docker/dockerfile:1.18
FROM golang:1.24-alpine AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .

ARG VERSION=dev
ARG COMMIT=unknown
ARG DATE=unknown
RUN CGO_ENABLED=0 go build \
    -trimpath \
    -ldflags="-s -w -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.date=${DATE}" \
    -o /outagedeck-prometheus-exporter \
    ./cmd/outagedeck-prometheus-exporter

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /outagedeck-prometheus-exporter /outagedeck-prometheus-exporter
EXPOSE 9787
USER nonroot:nonroot
ENTRYPOINT ["/outagedeck-prometheus-exporter"]
