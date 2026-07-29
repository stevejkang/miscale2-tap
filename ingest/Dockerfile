FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o /ingest .

FROM scratch
COPY --from=build /ingest /ingest
EXPOSE 51001
ENTRYPOINT ["/ingest"]
