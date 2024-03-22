FROM golang:1.22.1-alpine

# Set environment variables
ENV GO111MODULE=on

# Create app directory
RUN mkdir /app
WORKDIR /app

# Copy only necessary files
COPY go.mod .
COPY go.sum .

# Download necessary Go modules
RUN go mod download

# Copy the rest of the application source code
COPY . .

# Build the Go application
RUN go build -o app .

# Expose port 9011
EXPOSE 9011

# Command to run the application
CMD ["./app"]
