package testing

import (
	zip_streamer "github.com/scosman/zipstreamer/zip_streamer"
	"io/ioutil"
	"os"
	"testing"
)

var validFileEntry, _ = zip_streamer.NewFileEntry("https://pbs.twimg.com/media/DPRhf4ZX0AAFW1_.jpg", "mona.jpg")
var validFileEntry2, _ = zip_streamer.NewFileEntry("https://pbs.twimg.com/media/DPRhf4ZX0AAFW1_.jpg", "nested-folder/mona2.jpg")
var invalidFileEntry, _ = zip_streamer.NewFileEntry("https://pbs.twimg.com/media/fakeURL.jpg", "invalid.jpg")

func TestZipStreamCosntructorEmpty(t *testing.T) {
	z, err := zip_streamer.NewZipStream(make([]*zip_streamer.FileEntry, 0), ioutil.Discard)

	if err == nil || z != nil {
		t.Fatal("allowed empty streamer")
	}
}

func TestZipStreamCosntructor(t *testing.T) {
	z, err := zip_streamer.NewZipStream([]*zip_streamer.FileEntry{validFileEntry}, ioutil.Discard)

	if err != nil || z == nil {
		t.Fatal("constructor failed")
	}
}

const testFilePath = "test-out.zip"

func TestWriteZip(t *testing.T) {
	newfile, _ := os.Create(testFilePath)

	z, err := zip_streamer.NewZipStream([]*zip_streamer.FileEntry{validFileEntry, validFileEntry2}, newfile)
	if err != nil || z == nil {
		t.Fatal("constructor failed")
	}
	err = z.StreamAllFiles()
	if err != nil {
		t.Fatalf("issue writting zip: ", err)
	}
	newfile.Close()

	if info, _ := os.Stat(testFilePath); info.Size() == 0 {
		t.Fatal("output file is zero")
	}
}

const testFile2Path = "test-out2.zip"

func TestWriteZipWithSomeInvalid(t *testing.T) {
	newfile, _ := os.Create(testFile2Path)

	z, err := zip_streamer.NewZipStream([]*zip_streamer.FileEntry{validFileEntry, invalidFileEntry, validFileEntry2}, newfile)
	if err != nil || z == nil {
		t.Fatal("constructor failed")
	}
	err = z.StreamAllFiles()
	if err != nil {
		t.Fatalf("issue writting zip: ", err)
	}
	newfile.Close()

	if info, _ := os.Stat(testFile2Path); info.Size() == 0 {
		t.Fatal("output file is zero")
	}
}

const testFile3Path = "test-out3.zip"

func TestWriteZipWithAllInvalid(t *testing.T) {
	newfile, _ := os.Create(testFile3Path)

	z, err := zip_streamer.NewZipStream([]*zip_streamer.FileEntry{invalidFileEntry}, newfile)
	if err != nil || z == nil {
		t.Fatal("constructor failed")
	}
	err = z.StreamAllFiles()
	if err == nil {
		t.Fatalf("empty zip didn't error")
	}
	newfile.Close()
}
