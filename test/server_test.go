package testing

import (
  zip_streamer "github.com/scosman/zipstreamer/zip_streamer"
  "testing"
)

var invalidPayload = []byte(`{"entries": "dfdf"}`)
var emptyPayload = []byte(`{"entries": []}`)
var validPayload = []byte(`{"entries": [{"Url":"https://a.com/1","ZipPath":"file1.jpg"},{"Url":"https://a.com/2","ZipPath":"file2.jpg"}]}`)

func TestUnmarshalBodyInvalid(t *testing.T) {
  p, err := zip_streamer.UnmarshalPayload(invalidPayload)

  if err == nil || p != nil {
    t.Fatalf("allowed invalid payload: ", p)
  }
}

func TestUnmarshalBodyEmpty(t *testing.T) {
  r, err := zip_streamer.UnmarshalPayload(emptyPayload)

  if err != nil {
    t.Fatal("errored on empty payload")
  }

  if len(r) != 0 {
    t.Fatal("non-empty empty payload")
  }
}

func TestUnmarshalBodyValid(t *testing.T) {
  r, err := zip_streamer.UnmarshalPayload(validPayload)

  if err != nil {
    t.Fatalf("couldn't parse empty payload: %v", err)
  }
  if len(r) != 2 {
    t.Fatalf("incorrect entry count %v", len(r))
  }
  if r[0].Url().String() != "https://a.com/1" || r[1].ZipPath() != "file2.jpg" {
    t.Fatal("invalid parsing")
  }
}

