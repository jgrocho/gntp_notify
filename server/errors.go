package server

import (
	"strconv"
)

// GntpError represents GNTP error.
type GntpError struct {
	Code        int
	Description string
}

func (g GntpError) Error() string {
	return "GNTP " + strconv.Itoa(g.Code) + " error: " + g.Description
}

// Response builds a Response for the GntpError.
func (g GntpError) Response() *Response {
	resp := new(Response)
	resp.Version = Version{1, 0}
	resp.Type = "ERROR"
	header := NewHeader()
	header.Set("Error-Description", g.Description)
	header.Set("Error-Code", strconv.Itoa(g.Code))
	resp.Headers = make([]Header, 1)
	resp.Headers[0] = header
	return resp
}

func UnknownRequestTypeError(t string) GntpError {
	return GntpError{300, "Unknown or unsupported directive type: " + t}
}

func InvalidRequestError(info string) GntpError {
	return GntpError{300, "The request was malformed: " + info}
}

func UnknownProtocolError(p string) GntpError {
	return GntpError{301, "Unknown protocol: " + p}
}

func UnknownProtocolVersionError(v Version) GntpError {
	return GntpError{302, "Unknown protocol version: " + strconv.Itoa(v.Major) + "." + strconv.Itoa(v.Minor)}
}

func MissingHeaderError(header string) GntpError {
	return GntpError{303, "Required header " + header + " missing"}
}

func UnknownApplicationError(name string) GntpError {
	return GntpError{400, "Application " + name + " not known"}
}

func UnknownNotificationError(app, name string) GntpError {
	return GntpError{401, "Notification " + name + " not known for " + app}
}

func InternalServerError() GntpError {
	return GntpError{500, "The server encountered an internal error"}
}
