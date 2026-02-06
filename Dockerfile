FROM golang:1.25.5 AS builder 

WORKDIR /cli


COPY go.mod go.sum ./
RUN go mod download 

COPY . .
RUN go build -o server ./cmd/server


FROM debian:bookworm


# Install basic utilities needed for job execution
RUN apt-get update && apt-get install -y \
    ca-certificates \
    bash \
    coreutils \
    && rm -rf /var/lib/apt/lists/*

COPY --from=builder /cli/server .

ENTRYPOINT ["./server"]






