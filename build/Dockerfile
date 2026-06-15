FROM golang:1.26-alpine AS build
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /cosmos-operator ./cmd/cosmos-operator

FROM alpine:3.21
RUN adduser -D -u 65532 nonroot
COPY --from=build /cosmos-operator /usr/local/bin/cosmos-operator
USER 65532:65532
ENTRYPOINT ["cosmos-operator"]
