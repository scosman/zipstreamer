<h1 align="center"><img alt="Zipstreamer Logo" src="https://user-images.githubusercontent.com/848343/200652990-70e5f59d-c699-42ed-9a4f-a92cdfddda0d.png" width="300"></h1>

[![Build and Test](https://github.com/scosman/zipstreamer/actions/workflows/test.yml/badge.svg)](https://github.com/scosman/zipstreamer/actions/workflows/test.yml)
[![Format and Vet](https://github.com/scosman/zipstreamer/actions/workflows/format_check.yml/badge.svg)](https://github.com/scosman/zipstreamer/actions/workflows/format_check.yml)
[![Docker Generation](https://github.com/scosman/zipstreamer/actions/workflows/publish.yml/badge.svg)](https://github.com/scosman/zipstreamer/pkgs/container/packages%2Fzipstreamer)

**ZipStreamer** is a golang microservice for streaming zip files from a series of web links, on the fly. For example, if you have 200 files on S3, and you want to download a zip file of them, you can do so in 1 request to this server.

Highlights include:

 - Low memory: the files are streamed out to the client immediately
 - Low CPU: the default server doesn't compress files, only packages them into a zip, so there's minimal CPU load ([configurable](#customize-a))
 - High concurrency: the two properties above allow a single small server to stream hundreds of large zips simultaneous
 - [Easy to host](#deploy-a): several deployment options, including Docker images and two one-click deployers
 - It includes a HTTP server, but can be used as a library (see `zip_streamer.go`)

## Content

 - [JSON Zip File Descriptor](#json-descriptor-a)
 - [HTTP Endpoints](#http-endpoints-a)
   - [POST /download](#post-download-a)
   - [GET /download](#get-download-a)
   - [POST /create_download_link](#post-create-a)
   - [GET /download_link/{link_id}](#get-link-a)
 - [Deploy](#deploy-a)
   - [Heroku - One Click Deploy](#deploy-heroku-a)
   - [Google Cloud Run - One Click Deploy, Serverless](#deploy-google-a)
   - [Docker](#deploy-docker-a)
 - [Customization and Configuration](#customize-a)

<a name="json-descriptor-a"></a>
## JSON Zip File Descriptor 

Each HTTP endpoint requires a JSON description of the desired zip file. It includes a root object with the following structure:

 - `suggestedFilename` [optional, string]: The filename to suggest in the "Save As" UI in browsers. Defaults to `archive.zip` if not provided or invalid. [Limited to US-ASCII.](https://www.rfc-editor.org/rfc/rfc2183#section-2.3)
 - `files` [Required, array]: an array descibing the files to inclue in the zip file. Each array entry required 2 properties:
    - `url` [Required, string]: the URL of the file to include in the zip. Zipstreamer will fetch this via a GET request. The file must be publically accessible via this URL; most file hosts provide query string authentication options which work well with Zipstreamer (example [AWS S3 Docs](https://docs.aws.amazon.com/AmazonS3/latest/API/sigv4-query-string-auth.html)).
    - `zipPath`  [Required, string]: the path and filename where this entry should appear in the resulting zip file. This is a relative path to the root of the zip file.

Example JSON description with 2 files:

```
{
  "suggestedFilename": "tps_reports.zip",
  "files": [
    {
      "url":"https://server.com/image1.jpg",
      "zipPath":"image1.jpg"
    },
    {
      "url":"https://server.com/image2.jpg",
      "zipPath":"in-a-sub-folder/image2.jpg"
    }
  ]
}
```

<a name="http-endpoints-a"></a>
## HTTP Endpoints

<a name="post-download-a"></a>
### POST /download

This endpoint takes a http POST body containing the JSON zip file descriptor, and returns a zip file.

<a name="get-download-a"></a>
### GET /download

Returns a zip file, from a JSON zip file descriptor hosted on another server. This is useful over the POST endpoint in a few use cases:

 - You want to hide from the client where the original files are hosted (see `zsid` parameter)
 - Use cases where POST requests aren't easy to adopt (traditional static webpages)
 - You want to trigger a browsers' "Save File" UI, which isn't shown for POST requests. See `POST /create_download_link` if you prefer a client side method to achieve this.

This endpoint requires one of two query parameters describing where to find the JSON zip file descriptor:

 - `zsurl`: the full URL to the JSON file describing the zip. Example: `/download?zsurl=https://yourserver.com/path_to_descriptors/82a1b54cd20ab44a916bd76a5`
 - `zsid`: must be used with the `ZS_LISTFILE_URL_PREFIX` environment variable. The JSON file will be fetched from `ZS_LISTFILE_URL_PREFIX + zsid`. This allows you to hide the full URL path from clients, revealing only the end of the URL. Example: `ZS_LISTFILE_URL_PREFIX = "https://yoursever.com/path_to_descriptors/"` and `download?zsid=82a1b54cd20ab44a916bd76a5`

<a name="post-create-a"></a>
### POST /create_download_link

This endpoint takes the JSON zip file descriptor in the POST body, stores it in a local cache, returns a link ID which allows the caller to fetch the zip file via an additional call to `GET /download_link/{link_id}`.

This is useful for if you want to trigger a browser "Save File" UI, which isn't shown for POST requests. See `GET /download` if you prefer a server side method to achieve this.

*Important*:

 - These links only live for 60 seconds. They are expected to be used immediately.
 - This stores the link in an in-memory cache, so it's not suitable for deploying to a multi-server cluster without extra configuration. If you are hosting on a multi-server cluster, see the deployment section for configuration advice.

Here is an example response body containing the link ID. See docs for `GET /download_link/{link_id}` below for how to fetch this zip file:

```
{
  "status":"ok",
  "link_id":"b4ecfdb7-e0fa-4aca-ad87-cb2e4245c8dd"
}
```

<a name="get-link-a"></a>
### GET /download_link/{link_id}

Call this endpoint with a `link_id` generated with `/create_download_link` to download that zip file.

<a name="deploy-a"></a>
## Deploy

<a name="deploy-heroku-a"></a>
### Heroku - One Click Deploy

[![Deploy](https://www.herokucdn.com/deploy/button.svg)](https://heroku.com/deploy?template=https://github.com/scosman/zipstreamer/tree/master)

Be sure to enable [session affinity](https://devcenter.heroku.com/articles/session-affinity) if you're using multiple servers and using `/create_download_link`.

<a name="deploy-google-a"></a>
### Google Cloud Run - One Click Deploy, Serverless

[<img src="https://deploy.cloud.run/button.svg" width=180 alt="Run on Google Cloud">](https://deploy.cloud.run?git_repo=https%3A%2F%2Fgithub.com%2Fscosman%2Fzipstreamer)

Cloud Run is ideal serverless environment for ZipStreamer, as it routes many requests to a single container instance. ZipStreamer is designed to handle many concurrent requests, and will be cheaper to run on this serverless architecture than a instance-per-request architecture like AWS Lamba or Google Cloud Functions.

**Important** 
 - The one-click deploy button has [a bug](https://github.com/GoogleCloudPlatform/cloud-run-button/issues/232) and may force you to set the optional environment variables. If the server isn't working, check `ZS_URL_PREFIX` is blank in the Cloud Run console.
 - Be sure to enable [session affinity](https://cloud.google.com/run/docs/configuring/session-affinity) if you're using using `/create_download_link`. Cloud Run may scale up to multiple containers automatically.

<a name="deploy-docker-a"></a>
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

<a name="customize-a"></a>
## Customization and Configuration

These environment variables can be used to configure the server:

 - `PORT` - Defaults to 4008. Sets which port the HTTP server binds to.
 - `ZS_URL_PREFIX` - If set, the server will verify the `url` property of the files in the JSON zip file descriptors start with this prefix. Useful to preventing others from using your server to serve their files.
 - `ZS_COMPRESSION` - Defaults to no compression. It's not universally known, but zip files can be uncompressed, and used only to combining many files into one file. Set to `DEFLATE` to use zip deflate compression. **WARNING - enabling compression uses CPU, and will reduce throughput of server**. Note: for files with internal compression (JPEGs, MP4s, etc), zip DEFLATE compression will often increase the total zip file size.
  - `ZS_LISTFILE_URL_PREFIX` - See documentation for `GET /download`

## Why

I was mentoring at a "Teens Learning Code" class, but we had too many mentors, so I had some downtime.

## Logo

Zipper portion of logo by Kokota from Noun Project (Creative Commons CCBY)
