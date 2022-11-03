# syntax=docker/dockerfile:1
FROM golang:1.19-buster as builder

# Create and change to the app directory.
RUN echo 'DOCKERFILE: Setup'
WORKDIR /go/src/github.com/scosman/zipstreamer
COPY . .

# Build project
RUN echo 'DOCKERFILE: Building'
RUN go build

# Expose port - shouldn't override PORT env when using docker as server will bind to unexposed port. 
# Instead map exposed port 4008 to desired port when running. 
# Example to bind to port 80 `docker run -p 127.0.0.1:80:4008/tcp docker-zs`
ENV PORT=4008
EXPOSE 4008

RUN echo 'DOCKERFILE: Running'
CMD ["./zipstreamer"]