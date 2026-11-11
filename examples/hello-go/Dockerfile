FROM golang:1.22-alpine AS build
WORKDIR /src
COPY go.mod main.go ./
RUN CGO_ENABLED=0 go build -o /out/hello .

FROM alpine:3.20
WORKDIR /app
COPY --from=build /out/hello /app/hello
ENV PORT=8080
EXPOSE 8080
CMD ["/app/hello"]
