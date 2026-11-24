package zip_streamer

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"sort"
	"time"
	"unicode/utf8"
)

// ZipLayout describes a complete, uncompressed (store) zip archive whose
// byte layout is known before any file content is fetched. This is possible
// when size and CRC-32 of every file are provided in the descriptor (the same
// approach nginx mod_zip uses). Knowing the layout allows sending an exact
// Content-Length and serving arbitrary byte ranges of the archive.
type ZipLayout struct {
	segments []layoutSegment
	size     int64
	etag     string
}

// layoutSegment is a contiguous part of the archive: either static bytes
// (headers, central directory) or the content of one remote file.
type layoutSegment struct {
	offset int64
	size   int64
	data   []byte     // static bytes, nil for remote segments
	entry  *FileEntry // remote file, nil for static segments
}

const (
	uint16max = 0xffff
	uint32max = 0xffffffff

	localFileHeaderSig  = 0x04034b50
	centralDirHeaderSig = 0x02014b50
	endOfCentralDirSig  = 0x06054b50
	zip64EndOfDirSig    = 0x06064b50
	zip64LocatorSig     = 0x07064b50

	zipVersion20 = 20
	zipVersion45 = 45

	zip64ExtraID   = 0x0001
	extTimeExtraID = 0x5455

	flagUTF8 = 0x800
)

// DefaultModifiedTime is used for entries without an explicit modification
// time. A fixed value keeps the archive bytes stable across requests, which
// is required for resumable (range) downloads.
var DefaultModifiedTime = time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC)

var errMissingSizeOrCRC = errors.New("size and crc32 required for every file")

// CanBuildZipLayout reports whether all entries carry size and CRC-32.
func CanBuildZipLayout(entries []*FileEntry) bool {
	if len(entries) == 0 {
		return false
	}
	for _, e := range entries {
		if !e.HasSizeAndCRC32() {
			return false
		}
	}
	return true
}

// NewZipLayout computes the layout of a store-only zip archive for entries.
// Every entry must have size and CRC-32 set.
func NewZipLayout(entries []*FileEntry) (*ZipLayout, error) {
	if len(entries) == 0 {
		return nil, errors.New("must have at least 1 entry")
	}
	if !CanBuildZipLayout(entries) {
		return nil, errMissingSizeOrCRC
	}

	l := &ZipLayout{}
	var offset int64
	central := newByteBuilder()

	for _, e := range entries {
		size, _ := e.Size()
		crc, _ := e.CRC32()
		modified := e.Modified()
		if modified.IsZero() {
			modified = DefaultModifiedTime
		}
		name := e.ZipPath()
		if len(name) > uint16max {
			return nil, errors.New("zip path too long")
		}
		localOffset := offset

		flags := uint16(0)
		if !isASCII(name) && utf8.ValidString(name) {
			flags |= flagUTF8
		}
		sizeZip64 := size >= uint32max
		offsetZip64 := localOffset >= uint32max
		version := uint16(zipVersion20)
		if sizeZip64 || offsetZip64 {
			version = zipVersion45
		}
		dosDate, dosTime := timeToMsDosTime(modified)
		timeExtra := extendedTimestampExtra(modified)

		// Local file header
		lh := newByteBuilder()
		lh.uint32(localFileHeaderSig)
		lh.uint16(version)
		lh.uint16(flags)
		lh.uint16(0) // method: store
		lh.uint16(dosTime)
		lh.uint16(dosDate)
		lh.uint32(crc)
		var localExtra []byte
		if sizeZip64 {
			lh.uint32(uint32max) // compressed size
			lh.uint32(uint32max) // uncompressed size
			eb := newByteBuilder()
			eb.uint16(zip64ExtraID)
			eb.uint16(16)
			eb.uint64(uint64(size)) // uncompressed
			eb.uint64(uint64(size)) // compressed
			localExtra = eb.bytes()
		} else {
			lh.uint32(uint32(size))
			lh.uint32(uint32(size))
		}
		localExtra = append(localExtra, timeExtra...)
		lh.uint16(uint16(len(name)))
		lh.uint16(uint16(len(localExtra)))
		lh.write([]byte(name))
		lh.write(localExtra)

		l.appendStatic(&offset, lh.bytes())
		if size > 0 {
			l.segments = append(l.segments, layoutSegment{offset: offset, size: size, entry: e})
			offset += size
		}

		// Central directory header
		var centralExtra []byte
		cdSize, cdOffset := uint32(size), uint32(localOffset)
		if sizeZip64 || offsetZip64 {
			cdSize, cdOffset = uint32max, uint32max
			eb := newByteBuilder()
			eb.uint16(zip64ExtraID)
			eb.uint16(24)
			eb.uint64(uint64(size)) // uncompressed
			eb.uint64(uint64(size)) // compressed
			eb.uint64(uint64(localOffset))
			centralExtra = eb.bytes()
		}
		centralExtra = append(centralExtra, timeExtra...)
		central.uint32(centralDirHeaderSig)
		central.uint16(version) // version made by (MS-DOS)
		central.uint16(version) // version needed
		central.uint16(flags)
		central.uint16(0) // method: store
		central.uint16(dosTime)
		central.uint16(dosDate)
		central.uint32(crc)
		central.uint32(cdSize) // compressed
		central.uint32(cdSize) // uncompressed
		central.uint16(uint16(len(name)))
		central.uint16(uint16(len(centralExtra)))
		central.uint16(0) // comment length
		central.uint16(0) // disk number start
		central.uint16(0) // internal attrs
		central.uint32(0) // external attrs
		central.uint32(cdOffset)
		central.write([]byte(name))
		central.write(centralExtra)
	}

	centralStart := offset
	centralSize := int64(central.len())
	records := uint64(len(entries))
	end := central

	if records >= uint16max || centralSize >= uint32max || centralStart >= uint32max {
		zip64EndOffset := centralStart + centralSize
		// zip64 end of central directory record
		end.uint32(zip64EndOfDirSig)
		end.uint64(44) // size of remaining record
		end.uint16(zipVersion45)
		end.uint16(zipVersion45)
		end.uint32(0) // number of this disk
		end.uint32(0) // disk with central directory
		end.uint64(records)
		end.uint64(records)
		end.uint64(uint64(centralSize))
		end.uint64(uint64(centralStart))
		// zip64 end of central directory locator
		end.uint32(zip64LocatorSig)
		end.uint32(0) // disk with zip64 end record
		end.uint64(uint64(zip64EndOffset))
		end.uint32(1) // total number of disks

		records = uint16max
		centralSize = uint32max
		centralStart = uint32max
	}

	// end of central directory record
	end.uint32(endOfCentralDirSig)
	end.uint16(0) // number of this disk
	end.uint16(0) // disk with central directory
	end.uint16(uint16(records))
	end.uint16(uint16(records))
	end.uint32(uint32(centralSize))
	end.uint32(uint32(centralStart))
	end.uint16(0) // comment length

	l.appendStatic(&offset, end.bytes())
	l.size = offset
	l.etag = l.computeETag()
	return l, nil
}

func (l *ZipLayout) appendStatic(offset *int64, data []byte) {
	l.segments = append(l.segments, layoutSegment{offset: *offset, size: int64(len(data)), data: data})
	*offset += int64(len(data))
}

// Size returns the total size of the archive in bytes.
func (l *ZipLayout) Size() int64 {
	return l.size
}

// ETag returns a strong entity tag identifying the archive contents. It is
// derived from all headers (names, sizes, CRCs, timestamps), so it changes
// whenever the archive bytes would change. File URLs are not part of it, so
// rotating signed URLs keeps downloads resumable.
func (l *ZipLayout) ETag() string {
	return l.etag
}

func (l *ZipLayout) computeETag() string {
	h := sha256.New()
	var buf [8]byte
	for _, s := range l.segments {
		binary.LittleEndian.PutUint64(buf[:], uint64(s.size))
		h.Write(buf[:])
		if s.data != nil {
			h.Write(s.data)
		}
	}
	return `"` + hex.EncodeToString(h.Sum(nil)[:16]) + `"`
}

// segmentIndex returns the index of the segment containing pos.
func (l *ZipLayout) segmentIndex(pos int64) int {
	return sort.Search(len(l.segments), func(i int) bool {
		s := l.segments[i]
		return s.offset+s.size > pos
	})
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= utf8.RuneSelf {
			return false
		}
	}
	return true
}

// timeToMsDosTime converts t to MS-DOS date and time (2 second precision).
func timeToMsDosTime(t time.Time) (fDate uint16, fTime uint16) {
	t = t.UTC()
	if t.Year() < 1980 {
		t = time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC)
	} else if t.Year() > 2107 {
		t = time.Date(2107, 12, 31, 23, 59, 58, 0, time.UTC)
	}
	fDate = uint16(t.Day() + int(t.Month())<<5 + (t.Year()-1980)<<9)
	fTime = uint16(t.Second()/2 + t.Minute()<<5 + t.Hour()<<11)
	return
}

// extendedTimestampExtra returns the "UT" extra field carrying the exact
// UTC modification time, or nil if it doesn't fit in 32 bits.
func extendedTimestampExtra(t time.Time) []byte {
	unix := t.Unix()
	if unix < 0 || unix > uint32max {
		return nil
	}
	b := newByteBuilder()
	b.uint16(extTimeExtraID)
	b.uint16(5)
	b.uint8(1) // flags: modification time present
	b.uint32(uint32(unix))
	return b.bytes()
}

type byteBuilder struct {
	buf []byte
}

func newByteBuilder() *byteBuilder { return &byteBuilder{} }

func (b *byteBuilder) uint8(v uint8)   { b.buf = append(b.buf, v) }
func (b *byteBuilder) uint16(v uint16) { b.buf = binary.LittleEndian.AppendUint16(b.buf, v) }
func (b *byteBuilder) uint32(v uint32) { b.buf = binary.LittleEndian.AppendUint32(b.buf, v) }
func (b *byteBuilder) uint64(v uint64) { b.buf = binary.LittleEndian.AppendUint64(b.buf, v) }
func (b *byteBuilder) write(p []byte)  { b.buf = append(b.buf, p...) }
func (b *byteBuilder) bytes() []byte   { return b.buf }
func (b *byteBuilder) len() int        { return len(b.buf) }
