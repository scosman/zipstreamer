# syntax=docker/dockerfile:1
FROM golang:1.19-buster as builder

# Create and change to the app directory.
WORKDIR /go/src/github.com/scosman/zipstreamer
COPY . .

# Build project
RUN go build

# Use the official Debian slim image for a lean production container.
# https://hub.docker.com/_/debian
# https://docs.docker.com/develop/develop-images/multistage-build/#use-multi-stage-builds
FROM debian:buster-slim
RUN set -x && apt-get update && DEBIAN_FRONTEND=noninteractive apt-get install -y \
    ca-certificates && \
    rm -rf /var/lib/apt/lists/*

# Copy the binary to the production image from the builder stage.
COPY --from=builder /go/src/github.com/scosman/zipstreamer/zipstreamer /zipstreamer

# Expose port - shouldn't override PORT env when using docker as server will bind to unexposed port. 
# Instead map exposed port 4008 to desired port when running. 
# Example to bind to port 80 (assuming image named docker-zs) `docker run -p 127.0.0.1:80:4008/tcp docker-zs`
ENV PORT=4008
EXPOSE 4008

CMD ["/zipstreamer"]