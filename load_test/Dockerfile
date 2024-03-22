FROM golang:1.22.1-alpine AS build

WORKDIR /app
COPY . .

RUN go build -o load .


FROM alpine:latest

WORKDIR /app
COPY --from=build /app/load .

EXPOSE 9012

CMD ["./load"]