FROM golang:1.26-alpine AS build
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /cosmos ./cmd/cosmos

FROM alpine:3.19
RUN adduser -D -u 65532 nonroot
COPY --from=build /cosmos /usr/local/bin/cosmos
USER 65532:65532
ENTRYPOINT ["cosmos"]
