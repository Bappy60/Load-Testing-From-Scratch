# Build Stage
FROM golang:1.22.1-alpine AS build

WORKDIR /app

# Copy only necessary files for building
COPY . .

# Build the application
RUN go build -o load .

# Final Stage
FROM alpine:latest

WORKDIR /app

# Copy the built binary from the build stage
COPY --from=build /app/load .

# Expose port
EXPOSE 9012

# Command to run the application
CMD ["./load"]
