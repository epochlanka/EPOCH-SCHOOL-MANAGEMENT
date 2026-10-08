FROM golang:1.27 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /epoch-school .

FROM gcr.io/distroless/static-debian12
COPY --from=build /epoch-school /epoch-school
WORKDIR /app
ENV DB_PATH=/app/data/epoch.db
EXPOSE 8080
VOLUME ["/app/data"]
ENTRYPOINT ["/epoch-school"]
