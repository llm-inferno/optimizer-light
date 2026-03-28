# Use a multi-stage build
FROM golang:1.25-alpine AS builder
RUN apk update

WORKDIR /app
COPY . .

# Build all main.go files in cmd directory
RUN for file in $(find cmd -name "main.go"); do \
  dir=$(dirname "$file"); \
  name=$(basename "$dir"); \
  go build -o bin/$name $file; \
  done

# Create the final image
FROM alpine:3.20
RUN apk update
COPY --from=builder /app/bin /bin

# Expose the port the API will listen on
EXPOSE 8080

# Command to run the binary when the container starts
CMD ["optimizer"]