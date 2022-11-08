package testing

import (
	"testing"

	zip_streamer "github.com/scosman/zipstreamer/zip_streamer"
)

var invalidPayload = []byte(`{"entries": "dfdf"}`)
var emptyPayload = []byte(`{"entries": []}`)
var validPayload = []byte(`{"entries": [{"Url":"https://a.com/1","ZipPath":"file1.jpg"},{"Url":"https://a.com/2","ZipPath":"file2.jpg"}]}`)

func TestNewZipDescriptor(t *testing.T) {
	zd := zip_streamer.NewZipDescriptor()

	if len(zd.Files()) != 0 {
		t.Fatal("New zip descriptor has empty/invalid files")
	}

	if zd.EscapedSuggestedFilename() != "archive.zip" {
		t.Fatal("Default zip file name incorrect")
	}
}

func TestUnmarshalJsonInvalid(t *testing.T) {
	p, err := zip_streamer.UnmarshalJsonZipDescriptor(invalidPayload)

	if err == nil || p != nil {
		t.Fatalf("allowed invalid payload: %v", p)
	}
}

func TestUnmarshaJsonEmpty(t *testing.T) {
	r, err := zip_streamer.UnmarshalJsonZipDescriptor(emptyPayload)

	if err != nil {
		t.Fatal("errored on empty payload")
	}

	if len(r.Files()) != 0 {
		t.Fatal("non-empty empty payload")
	}
}

func TestUnmarshalJsonValid(t *testing.T) {
	r, err := zip_streamer.UnmarshalJsonZipDescriptor(validPayload)

	if err != nil {
		t.Fatalf("couldn't parse empty payload: %v", err)
	}
	if len(r.Files()) != 2 {
		t.Fatalf("incorrect entry count %v", len(r.Files()))
	}
	if r.Files()[0].Url().String() != "https://a.com/1" || r.Files()[1].ZipPath() != "file2.jpg" {
		t.Fatal("invalid parsing")
	}
}

var validPayloadWithArchiveNameNoExtension = []byte(`{"suggestedFilename": "customArchiveNameNoExtension", "entries": [{"Url":"https://a.com/1","ZipPath":"file1.jpg"},{"Url":"https://a.com/2","ZipPath":"file2.jpg"}]}`)

func TestUnmarshalJsonArchiveNameWithoutExtension(t *testing.T) {
	r, err := zip_streamer.UnmarshalJsonZipDescriptor(validPayloadWithArchiveNameNoExtension)
	if err != nil {
		t.Fatalf("couldn't parse empty payload: %v", err)
	}
	if r.EscapedSuggestedFilename() != "customArchiveNameNoExtension.zip" {
		t.Fatalf("Not appending zip suffix")
	}
}

var validPayloadWithArchiveNameWithExtension = []byte(`{"suggestedFilename": "customArchiveNameWithExtension.zip", "entries": [{"Url":"https://a.com/1","ZipPath":"file1.jpg"},{"Url":"https://a.com/2","ZipPath":"file2.jpg"}]}`)

func TestUnmarshalJsonArchiveNameWithExtension(t *testing.T) {
	r, err := zip_streamer.UnmarshalJsonZipDescriptor(validPayloadWithArchiveNameWithExtension)
	if err != nil {
		t.Fatalf("couldn't parse empty payload: %v", err)
	}
	if r.EscapedSuggestedFilename() != "customArchiveNameWithExtension.zip" {
		t.Fatalf("Not appending zip suffix")
	}
}

var validPayloadWithArchiveNameWithInvalidChars = []byte(`{"suggestedFilename": "Hello\"片😊.zip", "entries": [{"Url":"https://a.com/1","ZipPath":"file1.jpg"},{"Url":"https://a.com/2","ZipPath":"file2.jpg"}]}`)

func TestUnmarshalJsonArchiveNameWithInvalidChars(t *testing.T) {
	r, err := zip_streamer.UnmarshalJsonZipDescriptor(validPayloadWithArchiveNameWithInvalidChars)
	if err != nil {
		t.Fatalf("couldn't parse empty payload: %v", err)
	}
	if r.EscapedSuggestedFilename() != "Hello.zip" {
		t.Fatalf("Charater escaping failure: %v", r.EscapedSuggestedFilename())
	}
}

var validPayloadWithArchiveNameWithTooManyInvalidChars = []byte(`{"suggestedFilename": "\"片😊.zip", "entries": [{"Url":"https://a.com/1","ZipPath":"file1.jpg"},{"Url":"https://a.com/2","ZipPath":"file2.jpg"}]}`)

func TestUnmarshalJsonArchiveNameWithTooManyInvalidChars(t *testing.T) {
	r, err := zip_streamer.UnmarshalJsonZipDescriptor(validPayloadWithArchiveNameWithTooManyInvalidChars)
	if err != nil {
		t.Fatalf("couldn't parse empty payload: %v", err)
	}
	if r.EscapedSuggestedFilename() != "archive.zip" {
		t.Fatalf("Charater escaping failure: %v", r.EscapedSuggestedFilename())
	}
}
