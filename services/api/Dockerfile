FROM golang:1.25-alpine AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/api ./cmd/api \
    && CGO_ENABLED=0 GOOS=linux go build -o /out/worker ./cmd/worker

FROM gcr.io/distroless/static-debian12

COPY --from=build /out/api /api
COPY --from=build /out/worker /worker
EXPOSE 8080
ENTRYPOINT ["/api"]
