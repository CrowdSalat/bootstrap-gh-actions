FROM golang:1.24-alpine AS build
WORKDIR /src
COPY main.go .
RUN CGO_ENABLED=0 go build -o /app main.go

FROM scratch
COPY --from=build /app /app
USER 65534:0
ENTRYPOINT ["/app"]