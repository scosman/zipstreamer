FROM golang:1.15

WORKDIR /go/src/github.com/scosman/zipstreamer
COPY . .

RUN go build

# ENV DEFLATE=1
# ENV ZS_URL_PREFIX=
ENV PORT=4008
EXPOSE 4008

CMD ["./zipstreamer"]
