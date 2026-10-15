FROM golang:1.27.2-alpine3.23@sha256:9e45f0eb4a63ed37ad6e604950407558b7c738e34e015ae77a5d4ad369cde079 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /otel-redactor ./cmd/otel-redactor

FROM scratch
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build /otel-redactor /otel-redactor
COPY examples/container.yaml /etc/otel-redactor/config.yaml
USER 65532:65532
EXPOSE 4318
ENTRYPOINT ["/otel-redactor"]
CMD ["--config", "/etc/otel-redactor/config.yaml"]
