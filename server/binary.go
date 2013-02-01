package server

import (
	"bufio"
	"io"
	"net/textproto"
	"strconv"
	"strings"
)

// Binary represents binary data as read from a Request.
type Binary struct {
	Ident  string
	Length int64
	Data   []byte
}

// Objects implementing the Binaries interface allow for the saving and
// retrieval of binary data.
type Binaries interface {
	Add(key string, length int64, r io.Reader) error
	Get(key string) ([]byte, error)
	Exists(key string) bool
}

// ReadBinaries finds all the binary resource references found in
// headers, and saves them to binaries.
func ReadBinaries(b *bufio.Reader, headers []Header, binaries Binaries) (map[string]*Binary, error) {
	// Find how many header lines that have a value starting with the GNTP
	// resource identifier.
	count := 0
	for _, header := range headers {
		for _, values := range header {
			for _, value := range values {
				if strings.HasPrefix(value, "x-growl-resource://") {
					count += 1
				}
			}
		}
	}

	tp := textproto.NewReader(b)

	bs := make(map[string]*Binary, count)
	for i := 0; i < count; i++ {
		binary := new(Binary)
		// Read the Identifier and Length header block.
		header, err := tp.ReadMIMEHeader()
		if err != nil {
			return nil, err
		}

		if ident, ok := header["Identifier"]; ok {
			binary.Ident = ident[0]
		} else {
			return nil, MissingHeaderError("Binary Identifier")
		}

		if length, ok := header["Length"]; ok {
			if binary.Length, err = strconv.ParseInt(length[0], 10, 64); err != nil {
				return nil, InvalidRequestError(binary.Ident + " Length header invalid")
			}
		} else {
			return nil, MissingHeaderError("Length for binary " + binary.Ident)
		}

		// Read the data from b and add it to binaries.
		if err := binaries.Add(binary.Ident, binary.Length, b); err != nil && err == io.ErrUnexpectedEOF {
			return nil, InvalidRequestError(binary.Ident + " data incomplete")
		} else if err != nil {
			return nil, err
		}

		bs[binary.Ident] = binary

		// Read the two carriage-return/newlines at the end of the section.
		for i := 0; i < 2; i++ {
			var crlf byte
			if crlf, err = b.ReadByte(); err != nil {
				return nil, err
			}
			if crlf == '\r' {
				if crlf, err = b.ReadByte(); err != nil {
					return nil, err
				}
			}
			if crlf == '\n' {
				continue
			} else {
				// We should call b.UnreadByte() here, but we might have read
				// two bytes and we could only put one back in.
				return nil, InvalidRequestError(binary.Ident + " data not properly terminated")
			}
		}
	}

	return bs, nil
}
