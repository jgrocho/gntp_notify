package server

import (
	"fmt"
	"io"
	"net/textproto"
	"strings"
)

// Header represents a block of Header: Value lines.
type Header map[string][]string

// NewHeader allocates and initializes a Header.
func NewHeader() Header {
	return Header(make(map[string][]string))
}

// Add adds the key, value pair to the Header. It appends to any
// existing values associated with key.
func (h Header) Add(key, value string) {
	textproto.MIMEHeader(h).Add(key, value)
}

// Set sets the key, value pair in the Header. It overwrites any
// existing values associated with key.
func (h Header) Set(key, value string) {
	textproto.MIMEHeader(h).Set(key, value)
}

// Get gets the first value associated with key in the Header and
// returns it and true. If the key is unset it returns the empty string
// and false.
func (h Header) Get(key string) (string, bool) {
	v, ok := h[textproto.CanonicalMIMEHeaderKey(key)]
	if !ok {
		return "", false
	}
	return v[0], ok
}

// Del deletes the values associated with the key.
func (h Header) Del(key, value string) {
	textproto.MIMEHeader(h).Del(key)
}

// newlineToSpace replaces all newline characters with spaces, as these
// should not appear inside the value for a Header.
var newlineToSpace = strings.NewReplacer("\n", " ", "\r", " ")

// Write writes the Header to w.
func (h Header) Write(w io.Writer) error {
	for k, vs := range h {
		for _, v := range vs {
			v = newlineToSpace.Replace(v)
			v = strings.TrimSpace(v)
			if _, err := fmt.Fprintf(w, "%s: %s\r\n", k, v); err != nil {
				return err
			}
		}
	}
	return nil
}
