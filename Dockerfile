FROM --platform=$BUILDPLATFORM golang:1.25 AS builder

ARG TARGETOS
ARG TARGETARCH

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -o /out/nextddns ./cmd/nextddns

FROM gcr.io/distroless/static-debian12

WORKDIR /app
COPY --from=builder /out/nextddns /app/nextddns
EXPOSE 8080

ENTRYPOINT ["/app/nextddns"]
