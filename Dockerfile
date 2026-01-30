FROM golang:1.25.5 AS builder 

WORKDIR /cli

COPY . .

RUN go mod download 
RUN go build -o server ./cmd/server


FROM debian:bookworm

COPY --from=builder /cli/server .

ENTRYPOINT ["./server"]






