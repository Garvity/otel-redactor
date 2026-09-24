FROM golang:1.27.1-alpine3.23@sha256:0908ac9b9319e09d7c238aabe914e0395c51d63c4e3d0ae8c554fda9158a5769 AS build
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
