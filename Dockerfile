FROM golang:1.25 AS builder
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download || true
COPY --exclude=*_test.go . .
RUN CGO_ENABLED=0 go build -trimpath -o /virtiodp ./cmd/virtiodp

FROM gcr.io/distroless/static
COPY --from=builder /virtiodp /virtiodp
ENTRYPOINT ["/virtiodp"]
