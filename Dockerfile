FROM golang:1.22.1-alpine
ENV GO111MODULE=on

RUN mkdir /app
WORKDIR /app
ADD . /app
RUN apk add git

# Download necessary Go modules
COPY go.mod ./
COPY go.sum ./
RUN go mod download

COPY . .

RUN go build -o app .
EXPOSE 9011
CMD ["./app"]