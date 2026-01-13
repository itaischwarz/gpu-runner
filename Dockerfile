FROM golang:1.25.5 AS builder 

WORKDIR /cli

COPY . .

RUN go mod download 
RUN CGO_ENABLED=0 go build -o server ./cmd/server


FROM alpine:latest

COPY --from=builder /cli/server .

ENTRYPOINT ["./server"]






