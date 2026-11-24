package zip_streamer

import (
	"bytes"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"
)

type ZipDescriptor struct {
	suggestedFilenameRaw string
	files                []*FileEntry
}

func NewZipDescriptor() *ZipDescriptor {
	zd := ZipDescriptor{
		suggestedFilenameRaw: "",
		files:                make([]*FileEntry, 0),
	}

	return &zd
}

// Filename limited to US-ASCII per https://www.rfc-editor.org/rfc/rfc2183#section-2.3
// Filter " as it's used to quote this filename
func (zd ZipDescriptor) EscapedSuggestedFilename() string {
	rawFilename := zd.suggestedFilenameRaw
	escapedFilenameBuilder := make([]rune, 0, len(rawFilename))
	for _, r := range rawFilename {
		// Printable ASCII chars, no double quote
		if r > 31 && r < 127 && r != '"' {
			escapedFilenameBuilder = append(escapedFilenameBuilder, r)
		}
	}
	escapedFilename := string(escapedFilenameBuilder)

	if escapedFilename != "" && escapedFilename != ".zip" {
		if strings.HasSuffix(escapedFilename, ".zip") {
			return escapedFilename
		} else {
			return escapedFilename + ".zip"
		}
	}

	return "archive.zip"
}

func (zd ZipDescriptor) Files() []*FileEntry {
	return zd.files
}

// jsonCRC32 accepts a CRC-32 either as a hex string (as used by nginx mod_zip,
// e.g. "8a3f1b2c", optional "0x" prefix) or as a JSON number. An empty string
// or "-" means unknown.
type jsonCRC32 struct {
	value uint32
	set   bool
}

func (c *jsonCRC32) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)
	if bytes.Equal(data, []byte("null")) {
		return nil
	}
	if len(data) > 0 && data[0] == '"' {
		var s string
		if err := json.Unmarshal(data, &s); err != nil {
			return err
		}
		s = strings.TrimSpace(s)
		if s == "" || s == "-" {
			return nil
		}
		s = strings.TrimPrefix(strings.TrimPrefix(s, "0x"), "0X")
		v, err := strconv.ParseUint(s, 16, 32)
		if err != nil {
			return errors.New("invalid crc32 hex string")
		}
		c.value, c.set = uint32(v), true
		return nil
	}
	v, err := strconv.ParseUint(string(data), 10, 32)
	if err != nil {
		return errors.New("invalid crc32 number")
	}
	c.value, c.set = uint32(v), true
	return nil
}

type jsonZipEntry struct {
	DeprecatedCapitalizedUrl     string    `json:"Url"`
	DeprecatedCapitalizedZipPath string    `json:"ZipPath"`
	Url                          string    `json:"url"`
	ZipPath                      string    `json:"zipPath"`
	Size                         *int64    `json:"size"`
	CRC32                        jsonCRC32 `json:"crc32"`
	Modified                     string    `json:"modified"`
}

type jsonZipPayload struct {
	Files             []jsonZipEntry `json:"files"`
	DeprecatedEntries []jsonZipEntry `json:"entries"`
	SuggestedFilename string         `json:"suggestedFilename"`
}

func UnmarshalJsonZipDescriptor(payload []byte) (*ZipDescriptor, error) {
	var parsed jsonZipPayload
	err := json.Unmarshal(payload, &parsed)
	if err != nil {
		return nil, err
	}

	zd := NewZipDescriptor()
	zd.suggestedFilenameRaw = parsed.SuggestedFilename

	// Maintain backwards compatibility when files were named `entries`
	jsonZipFileList := parsed.Files
	if len(jsonZipFileList) == 0 {
		jsonZipFileList = parsed.DeprecatedEntries
	}

	for _, jsonZipFileItem := range jsonZipFileList {
		// Maintain backwards compatibility for non camel case parameters
		jsonFileItemUrl := jsonZipFileItem.Url
		if jsonFileItemUrl == "" {
			jsonFileItemUrl = jsonZipFileItem.DeprecatedCapitalizedUrl
		}
		jsonFileItemZipPath := jsonZipFileItem.ZipPath
		if jsonFileItemZipPath == "" {
			jsonFileItemZipPath = jsonZipFileItem.DeprecatedCapitalizedZipPath
		}

		fileEntry, err := NewFileEntry(jsonFileItemUrl, jsonFileItemZipPath)
		if err != nil {
			continue
		}
		if jsonZipFileItem.Size != nil {
			if err := fileEntry.SetSize(*jsonZipFileItem.Size); err != nil {
				continue
			}
		}
		if jsonZipFileItem.CRC32.set {
			fileEntry.SetCRC32(jsonZipFileItem.CRC32.value)
		}
		if jsonZipFileItem.Modified != "" {
			modified, err := time.Parse(time.RFC3339, jsonZipFileItem.Modified)
			if err != nil {
				continue
			}
			fileEntry.SetModified(modified)
		}
		zd.files = append(zd.files, fileEntry)
	}

	return zd, nil
}
