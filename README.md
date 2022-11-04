
[![Build and Test](https://github.com/scosman/zipstreamer/actions/workflows/test.yml/badge.svg)](https://github.com/scosman/zipstreamer/actions/workflows/test.yml)
[![Format and Vet](https://github.com/scosman/zipstreamer/actions/workflows/format_check.yml/badge.svg)](https://github.com/scosman/zipstreamer/actions/workflows/format_check.yml)
[![Docker Generation](https://github.com/scosman/zipstreamer/actions/workflows/publish.yml/badge.svg)](https://github.com/scosman/zipstreamer/pkgs/container/packages%2Fzipstreamer)

## About

ZipStreamer is a golang project for building and streaming zip files from a series of web links. For example, if you have 200 files on S3, and you want to download a zip file of them, you can do so in 1 request to this server.

Highlights include:

 - Low memory: the files are streamed out to the client immediately
 - Low CPU: the default server doesn't compress files, only packages them into a zip, so there's minimal CPU load (configurable)
 - High concurrency: the two properties above allow a single small server to stream hundreds of large zips simultaneous
 - It includes a HTTP server, but can be used as a library (see zip_streamer.go).

## HTTP Endpoints

**POST /download**

This endpoint takes a post, and returns a zip file.

It expects a JSON body defining which files to include in the zip. The `ZipPath` is the path and filename in the resulting zip file (it should be a relative path).

Example body:

```
{
  "entries": [
    {"Url":"https://server.com/image1.jpg","ZipPath":"image1.jpg"},
    {"Url":"https://server.com/image2.jpg","ZipPath":"in-a-sub-folder/image2.jpg"}
  ]
}
```

**POST /create_download_link**

This endpoint creates a temporary link which can be used to download a zip via a GET. This is helpful as on a webapp it can be painful to POST results, and trigger a "Save File" popup with the result. With this, you can create the link in a POST, then open the download link in a new window.

*Important*:

 - These links are only live for 60 seconds. They are expected to be used immediately and are not long living.
 - This stores the link in an in memory cache, so it's not suitable for deploying to a multi-server cluster without extra configuration. If you are hosting a multi-server cluster make sure to enable Session Affinity on your host, so that requests from a given client are routed to a consistent correct host. See the deploy section for details on Heroku and Google Cloud Run.

It expects the same body format as `/download`.

Here is an example response body:

```
{
  "status":"ok",
  "link_id":"b4ecfdb7-e0fa-4aca-ad87-cb2e4245c8dd"
}
```

**GET /download_link/{link_id}**

Call this endpoint with a `link_id` generated with `/create_download_link` to download that zip file.

## Deploy

### Heroku - One Click Deploy

[![Deploy](https://www.herokucdn.com/deploy/button.svg)](https://heroku.com/deploy?template=https://github.com/scosman/zipstreamer/tree/master)

Be sure to enable [session affinity](https://devcenter.heroku.com/articles/session-affinity) if you're using multiple servers and using `/create_download_link`.

### Google Cloud Run - One Click Deploy, Serverless

[![Run on Google Cloud](https://deploy.cloud.run/button.svg)](https://deploy.cloud.run?git_repo=https%3A%2F%2Fgithub.com%2Fscosman%2Fzipstreamer)

**Important** 
 - The one-click deploy button has [a bug](https://github.com/GoogleCloudPlatform/cloud-run-button/issues/232) and may force you to set the optional environment variables. If the server isn't working, check `ZS_URL_PREFIX` is blank in the Cloud Run console.
 - Be sure to enable [session affinity](https://cloud.google.com/run/docs/configuring/session-affinity) if you're using using `/create_download_link`. Cloud Run may scale up to multiple containers automatically.

Cloud Run is ideal for zipstreamer, as it routes many requests to a single container instance. Zipstreamer is designed to handle many concurrent requests, and will be cheaper to run on this serverless architecture than a instance-per-request architecture like AWS Lamba or Google Cloud Functions.

### Docker 

This repo contains an dockerfile, and an image is published [on Github Packages](https://github.com/scosman/zipstreamer/pkgs/container/packages%2Fzipstreamer).

#### Build Your Own Image

To build your own image, clone the repo and run: 

```
docker build --tag docker-zipstreamer .
# Start on port 8080
docker run --env PORT=8080 -p 8080:8080 docker-zipstreamer
```

#### Run Offical Package from Github Packages

Currently every change to master it published as a package. To use these offical packages:

```
docker pull ghcr.io/scosman/packages/zipstreamer:latest
# Start on port 8080
docker run --env PORT=8080 -p 8080:8080 ghcr.io/scosman/packages/zipstreamer:latest
```

## Config

These ENV vars can be used to config the server:

 - `PORT` - Defaults to 4008. Sets which port the HTTP server binds to.
 - `ZS_URL_PREFIX` - If set, requires that the URL of files downloaded start with this prefix. Useful to preventing others from using your server to serve their files.
 - `ZS_COMPRESSION` - Defaults to no compression. It's not universally known, but zip files can be uncompressed, and used as a simple packaging format (combined many files into one). Set to `DEFLATE` to use zip deflate compression. **WARNING - enabling compression uses CPU, and will greatly reduce throughput of server**. Note: for file formats already optimized for size (JPEGs, MP4s), zip compression will often increase the total zip file size.

## Why

I was mentoring at a "Teens Learning Code" class, but we had too many mentors, so I had some downtime.

