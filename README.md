
[![Build and Test](https://github.com/scosman/zipstreamer/actions/workflows/test.yml/badge.svg)](https://github.com/scosman/zipstreamer/actions/workflows/test.yml)
[![Format and Vet](https://github.com/scosman/zipstreamer/actions/workflows/format_check.yml/badge.svg)](https://github.com/scosman/zipstreamer/actions/workflows/format_check.yml)
[![Docker Generation](https://github.com/scosman/zipstreamer/actions/workflows/publish.yml/badge.svg)](https://github.com/scosman/zipstreamer/pkgs/container/packages%2Fzipstreamer)

## About

ZipStreamer is a golang project for building and streaming zip files from a series of web links. For example, if you have 200 files on S3, and you want to download a zip file of them, you can do so in 1 request to this server.

Highlights include:

 - Low memory: the files are streamed out to the client immediately
 - Low CPU: the default server doesn't compress files, only packages them into a zip, so there's minimal CPU load (configurable)
 - High concurrency: the two properties above allow a single small server to stream hundreds of large zips simultaneous
 - It includes a HTTP server, but can be used as a library (see `zip_streamer.go`)

## JSON Zip File Descriptor

Each HTTP endpoint requires a JSON description of the desired zip file. It includes a root object with the following structure:

 - `suggestedFilename` [optional, string]: The filename to suggest in the "Save As" UI in browsers. Defaults to `archive.zip` if not provided or invalid. [Limited to US-ASCII.](https://www.rfc-editor.org/rfc/rfc2183#section-2.3)
 - `entries` [Required, array]: an array descibing the files to inclue in the zip file. Each array entry required 2 properties:
    - `Url` [Required, string]: the URL of the file to include in the zip. Zipstreamer will fetch this via a GET request. The file must be public; if it is private, most file hosts provide query string authentication options for private files, which work well with Zipstreamer (example [AWS S3 Docs](https://docs.aws.amazon.com/AmazonS3/latest/API/sigv4-query-string-auth.html)).
    - `ZipPath`  [Required, string]: the path and filename where this entry should appear in the resulting zip file. This is a relative path to the root of the zip file.

Example JSON description with 2 files:

```
{
  "suggestedFilename": "tps_reports.zip",
  "entries": [
    {
      "Url":"https://server.com/image1.jpg",
      "ZipPath":"image1.jpg"
    },
    {
      "Url":"https://server.com/image2.jpg",
      "ZipPath":"in-a-sub-folder/image2.jpg"
    }
  ]
}
```

## HTTP Endpoints

### POST /download

This endpoint takes a http POST body containing the JSON description of the desired zip file, and returns a zip file.

### GET /download

Returns a zip file, from a JSON zip description hosted on another server. This is useful over the POST endpoint in a few use cases:

 - You want to hide from the client where the original files are hosted (see zsid parameter)
 - Use cases where POST requests aren't easy to adopt (traditional static webpages)
 - You want to trigger a browsers' "Save File" UI, which isn't shown for POST requests. See `POST /create_download_link` as an alternative if you prefer writing this logic client side.

This endpoint requires one of two query parameters describing where to find the JSON descriptor. If both are provided, only `zsurl` will be used:

 - `zsurl`: the full URL to the JSON file describing the zip. Example: `zipstreamer.yourserver.com/download?zsurl=https://gist.githubusercontent.com/scosman/449df713f97888b931c7b4e4f76f82b1/raw/82a1b54cd20ab44a916bd76a5b5d866acee2b29a/listfile.json`
 - `zsid`: must be used with the `ZS_LISTFILE_URL_PREFIX` environment variable. The JSON file will be fetched from `ZS_LISTFILE_URL_PREFIX + zsid`. This allows you to hide the full URL path from clients, revealing only the end of the URL. Example: `ZS_LISTFILE_URL_PREFIX = "https://gist.githubusercontent.com/scosman/"` and `zipstreamer.yourserver.com/download?zsid=449df713f97888b931c7b4e4f76f82b1/raw/82a1b54cd20ab44a916bd76a5b5d866acee2b29a/listfile.json`

### POST /create_download_link

This endpoint takes the JSON zip description in the POST body, stores it in a local cache, allowing the caller to fetch the zip file via an additional call to `GET /download_link/{link_id}`.

This is useful for if you want to trigger a browser "Save File" UI, which isn't shown for POST requests. See `GET /download` if you prefer a server-driven approach.

*Important*:

 - These links only live for 60 seconds. They are expected to be used immediately.
 - This stores the link in an in-memory cache, so it's not suitable for deploying to a multi-server cluster without extra configuration. If you are hosting a multi-server cluster, see the deployment section for options.

Here is an example response body containing the link ID. See docs for `GET /download_link/{link_id}` below for how to fetch the zip file:

```
{
  "status":"ok",
  "link_id":"b4ecfdb7-e0fa-4aca-ad87-cb2e4245c8dd"
}
```

### GET /download_link/{link_id}

Call this endpoint with a `link_id` generated with `/create_download_link` to download that zip file.

## Deploy

### Heroku - One Click Deploy

[![Deploy](https://www.herokucdn.com/deploy/button.svg)](https://heroku.com/deploy?template=https://github.com/scosman/zipstreamer/tree/master)

Be sure to enable [session affinity](https://devcenter.heroku.com/articles/session-affinity) if you're using multiple servers and using `/create_download_link`.

### Google Cloud Run - One Click Deploy, Serverless

[<img src="https://deploy.cloud.run/button.svg" width=180 alt="Run on Google Cloud">](https://deploy.cloud.run?git_repo=https%3A%2F%2Fgithub.com%2Fscosman%2Fzipstreamer)

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

#### Run Official Package from Github Packages

Official packages are published on Github packages. To pull latest stable release:

```
docker pull ghcr.io/scosman/packages/zipstreamer:stable
# Start on port 8080
docker run --env PORT=8080 -p 8080:8080 ghcr.io/scosman/packages/zipstreamer:stable
```

Note: `stable` pulls the latest github release. Use `ghcr.io/scosman/packages/zipstreamer:latest` for top of tree.

## Config

These ENV vars can be used to config the server:

 - `PORT` - Defaults to 4008. Sets which port the HTTP server binds to.
 - `ZS_URL_PREFIX` - If set, requires that the URL of files downloaded start with this prefix. Useful to preventing others from using your server to serve their files.
 - `ZS_COMPRESSION` - Defaults to no compression. It's not universally known, but zip files can be uncompressed, and used as a simple packaging format (combined many files into one). Set to `DEFLATE` to use zip deflate compression. **WARNING - enabling compression uses CPU, and will greatly reduce throughput of server**. Note: for file formats already optimized for size (JPEGs, MP4s), zip compression will often increase the total zip file size.
  - `ZS_LISTFILE_URL_PREFIX` - See documentation for `GET /download`

## Why

I was mentoring at a "Teens Learning Code" class, but we had too many mentors, so I had some downtime.

