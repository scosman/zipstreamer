package testing

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	zip_streamer "github.com/scosman/zipstreamer/zip_streamer"
)

func checkHttpOk(rr *httptest.ResponseRecorder, t *testing.T) {
	if status := rr.Code; status != http.StatusOK {
		t.Fatalf("handler returned wrong status code: got %v want %v",
			status, http.StatusOK)
	}
}

// same as stored at https://gist.githubusercontent.com/scosman/449df713f97888b931c7b4e4f76f82b1/raw/b2f6fe60874d5183d2a2ec689c574b66751cf4ae/listfile.json
var jsonDescriptorToValidate = []byte(`{
	"suggestedFilename":"download.zip",
	"files": [
	  {
		"url":"https://gist.githubusercontent.com/scosman/449df713f97888b931c7b4e4f76f82b1/raw/d97b5e7c1a9dcbf567938ae4914f1bb7f2dd0290/listfile.json",
		"zipPath":"file1.json"
	  },{
		"url":"https://gist.githubusercontent.com/scosman/449df713f97888b931c7b4e4f76f82b1/raw/d97b5e7c1a9dcbf567938ae4914f1bb7f2dd0290/listfile.json",
		"zipPath":"subfolder/file2.json"}
	]
  }`)

func checkResponseZipFile(rr *httptest.ResponseRecorder, t *testing.T) {
	// Check headers
	if rr.Result().Header.Get("Content-Type") != "application/zip" {
		t.Fatalf("Not zip content type")
	}
	if rr.Result().Header.Get("Content-Disposition") != "attachment; filename=\"download.zip\"" {
		t.Errorf("Attachment name incorrect: %v", rr.Result().Header.Get("Content-Disposition"))
	}

	// Read and validate zip file
	zipBytes := rr.Body.Bytes()
	zipReader, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil {
		t.Fatalf("General error -- tests require network connection which might or might not be the problem, %v", err)
	}
	if len(zipReader.File) != 2 {
		t.Errorf("Expected exactly 2 files in zip")
	}
	if zipReader.File[0].FileHeader.Name != "file1.json" {
		t.Errorf("Expected file 1 to have filename file1.json, got: %v", zipReader.File[0].FileHeader.Name)
	}
	if zipReader.File[1].FileHeader.Name != "subfolder/file2.json" {
		t.Errorf("Expected file 2 to have filename file2.json, got: %v", zipReader.File[1].FileHeader.Name)
	}
	if zipReader.File[0].FileHeader.FileInfo().Size() != 110 {
		t.Errorf("Expected file 1 to have size 110, got: %v", zipReader.File[0].FileHeader.FileInfo().Size())
	}
	if zipReader.File[1].FileHeader.FileInfo().Size() != 110 {
		t.Errorf("Expected file 2 to have size 110, got: %v", zipReader.File[1].FileHeader.FileInfo().Size())
	}
}

func TestServerGetDownloadZsurl(t *testing.T) {
	req, err := http.NewRequest("GET", "/download?zsurl=https://gist.githubusercontent.com/scosman/449df713f97888b931c7b4e4f76f82b1/raw/b2f6fe60874d5183d2a2ec689c574b66751cf4ae/listfile.json", nil)
	if err != nil {
		t.Fatal(err)
	}

	zipServer := zip_streamer.NewServer()
	rr := httptest.NewRecorder()
	zipServer.ServeHTTP(rr, req)

	checkHttpOk(rr, t)
	checkResponseZipFile(rr, t)
}

func TestServerGetDownloadZsid(t *testing.T) {
	req, err := http.NewRequest("GET", "/download?zsid=449df713f97888b931c7b4e4f76f82b1/raw/b2f6fe60874d5183d2a2ec689c574b66751cf4ae/listfile.json", nil)
	if err != nil {
		t.Fatal(err)
	}

	zipServer := zip_streamer.NewServer()
	zipServer.ListfileUrlPrefix = "https://gist.githubusercontent.com/scosman/"
	rr := httptest.NewRecorder()
	zipServer.ServeHTTP(rr, req)

	checkHttpOk(rr, t)
	checkResponseZipFile(rr, t)
}

func TestServerPostDownload(t *testing.T) {
	req, err := http.NewRequest("POST", "/download", bytes.NewReader(jsonDescriptorToValidate))
	if err != nil {
		t.Fatal(err)
	}

	zipServer := zip_streamer.NewServer()
	rr := httptest.NewRecorder()
	zipServer.ServeHTTP(rr, req)

	checkHttpOk(rr, t)
	checkResponseZipFile(rr, t)
}

type jsonCreateLinkPayload struct {
	Status string `json:"status"`
	LinkId string `json:"link_id"`
}

func TestServerCreateAndGet(t *testing.T) {
	createReq, err := http.NewRequest("POST", "/create_download_link", bytes.NewReader(jsonDescriptorToValidate))
	if err != nil {
		t.Fatal(err)
	}

	zipServer := zip_streamer.NewServer()
	rrCreate := httptest.NewRecorder()
	zipServer.ServeHTTP(rrCreate, createReq)
	checkHttpOk(rrCreate, t)

	var createResponseParsed jsonCreateLinkPayload
	err = json.Unmarshal(rrCreate.Body.Bytes(), &createResponseParsed)
	if err != nil || createResponseParsed.Status != "ok" {
		t.Fatalf("Create link request failed (%v): %v", createResponseParsed.Status, err)
	}

	getReq, err := http.NewRequest("GET", "/download_link/"+createResponseParsed.LinkId, nil)
	if err != nil {
		t.Fatal(err)
	}

	rrGet := httptest.NewRecorder()
	zipServer.ServeHTTP(rrGet, getReq)

	checkHttpOk(rrGet, t)
	checkResponseZipFile(rrGet, t)
}

func TestServerCreateLinkNoFiles(t *testing.T) {
	emptyFilesJson := []byte(`{
		"suggestedFilename":"download.zip",
		"files": []
	}`)

	req, err := http.NewRequest("POST", "/create_download_link", bytes.NewReader(emptyFilesJson))
	if err != nil {
		t.Fatal(err)
	}

	zipServer := zip_streamer.NewServer()
	rr := httptest.NewRecorder()
	zipServer.ServeHTTP(rr, req)

	// Check for bad request status
	if status := rr.Code; status != http.StatusBadRequest {
		t.Fatalf("handler returned wrong status code: got %v want %v",
			status, http.StatusBadRequest)
	}

	// Verify error message
	var response struct {
		Status string `json:"status"`
		Error  string `json:"error"`
	}
	err = json.Unmarshal(rr.Body.Bytes(), &response)
	if err != nil {
		t.Fatalf("Failed to parse response JSON: %v", err)
	}

	expectedError := "no files to download"
	if response.Status != "error" || response.Error != expectedError {
		t.Errorf("Expected error response {status: 'error', error: '%s'}, got {status: '%s', error: '%s'}",
			expectedError, response.Status, response.Error)
	}
}
