package zip_streamer

import (
  "github.com/gorilla/mux"
  "net/http"
  "encoding/json"
  "io/ioutil"
  "time"
  "github.com/google/uuid"
)

type zipEntry struct {
  Url string
  ZipPath string
}

type zipPayload struct {
  Entries []zipEntry
}

func UnmarshalPayload(payload []byte) ([]*FileEntry, error) {
  var parsed zipPayload
  err := json.Unmarshal(payload, &parsed)
  if err != nil {
    return nil, err
  }

  results := make([]*FileEntry, 0)
  for _, entry := range parsed.Entries {
    fileEntry, err := NewFileEntry(entry.Url, entry.ZipPath)
    if err == nil {
      results = append(results, fileEntry)
    }
  }

  return results, nil
}

type Server struct {
  router *mux.Router
  linkCache LinkCache
}

func NewServer() (*Server) {
  r := mux.NewRouter()

  timeout := time.Second * 60
  server := Server{
    router: r,
    linkCache: NewLinkCache(&timeout),
  }

  r.HandleFunc("/download", server.HandlePostDownload).Methods("POST")
  r.HandleFunc("/create_download_link", server.HandleCreateLink).Methods("POST")
  r.HandleFunc("/download_link/{link_id}", server.HandleDownloadLink).Methods("GET")

  return &server
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
  s.router.ServeHTTP(w, r)
}

func (s *Server) HandleCreateLink(w http.ResponseWriter, req *http.Request) {
  fileEntries, err := s.parseZipRequest(w, req)
  if err != nil {
    return
  }

  linkId := uuid.New().String()
  s.linkCache.Set(linkId, fileEntries)
  w.WriteHeader(http.StatusOK)
  w.Write([]byte(`{"status":"ok","link_id":"` + linkId + `"}`))
}

func (s *Server) parseZipRequest(w http.ResponseWriter, req *http.Request) ([]*FileEntry, error) {
  body, err := ioutil.ReadAll(req.Body)
  if err != nil {
    w.WriteHeader(http.StatusBadRequest)
    w.Write([]byte(`{"status":"error","error":"missing body"}`))
    return nil, err
  }

  fileEntries, err := UnmarshalPayload(body)
  if err != nil {
    w.WriteHeader(http.StatusBadRequest)
    w.Write([]byte(`{"status":"error","error":"invalid body"}`))
    return nil, err
  }

  return fileEntries, nil
}

func (s *Server) HandlePostDownload(w http.ResponseWriter, req *http.Request) {
  fileEntries, err := s.parseZipRequest(w, req)
  if err != nil {
    return
  }

  s.streamEntries(fileEntries, w, req)
}

func (s *Server) HandleDownloadLink(w http.ResponseWriter, req *http.Request) {
  linkId := mux.Vars(req)["link_id"]
  fileEntries := s.linkCache.Get(linkId)
  if fileEntries == nil {
    w.WriteHeader(http.StatusNotFound)
    w.Write([]byte(`{"status":"error","error":"link not found"}`))
    return
  }

  s.streamEntries(fileEntries, w, req)
}

func (s *Server) streamEntries(fileEntries []*FileEntry, w http.ResponseWriter, req *http.Request) {
  zipStreamer, err := NewZipStream(fileEntries, w)
  if err != nil {
    w.WriteHeader(http.StatusBadRequest)
    w.Write([]byte(`{"status":"error","error":"invalid entries"}`))
    return
  }

  // need to write the header before bytes
  w.Header().Set("Content-Type", "application/zip")
  w.Header().Set("Content-Disposition", "attachment; filename=\"archive.zip\"")
  w.WriteHeader(http.StatusOK)
  err = zipStreamer.StreamAllFiles()
  if err != nil {
    w.Write([]byte(`{"status": "error", "error": "internal error"}`))
  }
}

