package testing

import (
  zip_streamer "github.com/scosman/zipstreamer/zip_streamer"
  "testing"
  "path"
  "os"
)

const validUrl = "https://pbs.twimg.com/media/DPRhf4ZX0AAFW1_.jpg"

func TestFileEntry(t *testing.T) {
  f, err := zip_streamer.NewFileEntry(validUrl, "mona.jpg")

  if err != nil {
    t.Fatal("File entry constructor failed")
  }

  if f.ZipPath() != "mona.jpg" {
    t.Fatal("path mismatch")
  }
}

func TestFileEntryInvalidUrl(t *testing.T) {
  f, err := zip_streamer.NewFileEntry("ftp://sdfs/sdf.c", "mona.jpg")

  if err == nil || f != nil {
    t.Fatal("accpeted non web url")
  }

  f, err = zip_streamer.NewFileEntry("/sdfs/sdf.c", "mona.jpg")

  if err == nil || f != nil {
    t.Fatal("accpeted local url")
  }
}

func TestFileEntryInvalidAbsPath(t *testing.T) {
  f, err := zip_streamer.NewFileEntry(validUrl, "/mona.jpg")

  if err == nil || f != nil {
    t.Fatal("accepted abs path")
  }
}

func TestFileEntryPath(t *testing.T) {
  f, err := zip_streamer.NewFileEntry(validUrl, "folder//mona.jpg")

  if err != nil {
    t.Fatal("didn't accept subpath")
  }

  if f.ZipPath() != "folder/mona.jpg" {
    t.Fatal("didn't clean path")
  }
}

func TestFileEntryInvalidPath(t *testing.T) {
  f, err := zip_streamer.NewFileEntry(validUrl, "")

  if err == nil || f != nil {
    t.Log(path.Split(f.ZipPath()))
    t.Fatal("accepted empty path")
  }
}

func TestFilePrefix(t *testing.T) {
  origVal := os.Getenv(zip_streamer.UrlPrefixEnvVar)
  defer os.Setenv(zip_streamer.UrlPrefixEnvVar, origVal)

  os.Setenv(zip_streamer.UrlPrefixEnvVar, "https://pbs.twimg.com")
  _, err := zip_streamer.NewFileEntry(validUrl, "mona.jpg")
  if err != nil {
    t.Fatal("prefix match failed")
  }

  os.Setenv(zip_streamer.UrlPrefixEnvVar, "https://google.com")
  f, err := zip_streamer.NewFileEntry(validUrl, "mona.jpg")
  if err == nil || f != nil {
    t.Fatal("prefix block failed")
  }
}

