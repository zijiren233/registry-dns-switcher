FROM golang:1.25 AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /registry-dns-switcher .

FROM gcr.io/distroless/static:nonroot
COPY --from=builder /registry-dns-switcher /registry-dns-switcher
USER nonroot:nonroot
ENTRYPOINT ["/registry-dns-switcher"]
