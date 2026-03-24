FROM golang:1.25 AS builder
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download || true
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /virtiodp ./cmd/virtiodp

FROM gcr.io/distroless/static:nonroot
COPY --from=builder /virtiodp /virtiodp
ENTRYPOINT ["/virtiodp"]
