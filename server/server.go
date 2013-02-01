/*
Package server provides a GNTP server implementation.

It is intended to be used as the basis for processing GNTP requests and
sending replies. Its design and implementation is inspired by Go's http
server library.

Users of this library should register handlers for different GNTP request
types, then start the server.

	server.Register("REGISTER", registerHandler)
	server.Start()

The server should be gracefully shutdown with a call to Exit:

	server.Exit()
*/
package server

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"log"
	"net"
	"net/textproto"
	"runtime/debug"
	"strings"
	"sync"
	"time"
)

// Version represents a GNTP Version.
type Version struct {
	Major int
	Minor int
}

// String returns the GNTP version identifier for v.
func (v Version) String() string {
	return fmt.Sprintf("GNTP/%d.%d", v.Major, v.Minor)
}

// Request represents a GNTP request.
type Request struct {
	Version  Version            // the GNTP version
	Type     string             // the type of request (REGISTER, NOTIFY, etc.)
	Headers  []Header           // a slice of Header lines
	Binaries map[string]*Binary // a map from Identifier to Binary data
}

// Response represents a GNTP response
type Response struct {
	Version  Version            // the GNTP version
	Type     string             // the type of response (OK, ERROR, etc.)
	Headers  []Header           // a slice of Header lines
	Binaries map[string]*Binary // a map from Identifier to Binary data
}

// NewResponse creates a Response with the specified major and minor
// version.
func NewResponse(major, minor int) *Response {
	resp := new(Response)
	resp.Version.Major = major
	resp.Version.Minor = minor
	resp.Type = "OK"
	resp.Headers = make([]Header, 1, 32)
	resp.Headers[0] = Header(make(map[string][]string))
	resp.Binaries = make(map[string]*Binary)
	return resp
}

// write formats and writes resp to the given io.Writer.
func (resp *Response) write(w io.Writer) error {
	tp := textproto.NewWriter(bufio.NewWriter(w))

	// Write the GNTP directive line.
	if err := tp.PrintfLine("GNTP/%d.%d -%s NONE",
		resp.Version.Major, resp.Version.Minor,
		resp.Type); err != nil {
		return err
	}

	// Write each block of header lines.
	for _, header := range resp.Headers {
		if err := header.Write(w); err != nil {
			return err
		}
		// ...ending with a blank line.
		if err := tp.PrintfLine(""); err != nil {
			return err
		}
	}

	// Write each binary
	for id, binary := range resp.Binaries {
		if err := tp.PrintfLine("Identifier: %s", id); err != nil {
			return err
		}
		if err := tp.PrintfLine("Length: %d", binary.Length); err != nil {
			return err
		}
		if _, err := w.Write(binary.Data); err != nil {
			return err
		}
		if err := tp.PrintfLine(""); err != nil {
			return err
		}
	}

	return nil
}

// Object implementing the Handler interface register to parse and then
// respond to GNTP requests.
//
// Parse should read as much from the bufio.Reader as it needs and
// return a new or modified Request. Anything read by a previous Parse
// will be in the passed-in Request.
// Respond takes a Request and generates a Response.
type Handler interface {
	Parse(*bufio.Reader, *Request) (*Request, error)
	Respond(*Request) (*Response, error)
}

// ServeMux is a GNTP request multiplexer. It matches the type of an
// incoming request and calls the handler registered for that type.
//
// It parses the directive line only.
type ServeMux struct {
	mu sync.RWMutex
	m  map[string]Handler
}

// NewServeMux allocates and returns a new ServeMux.
func NewServeMux() *ServeMux {
	return &ServeMux{m: make(map[string]Handler)}
}

// DefaultServeMux is the default ServeMux used by Start.
var DefaultServeMux = NewServeMux()

// Register registers the the Handler h for Requests of Type t.
func (mux *ServeMux) Register(t string, h Handler) {
	mux.mu.Lock()
	defer mux.mu.Unlock()

	if _, defined := mux.m[t]; defined {
		panic("gntp: multiple registrations for " + t)
	}

	mux.m[t] = h
}

// Register registers the the Handler h for Requests of Type t for the
// DefaultServeMux.
func Register(pattern string, h Handler) {
	DefaultServeMux.Register(pattern, h)
}

// handler gets the handler for the given Request Type.
func (mux *ServeMux) handler(t string) Handler {
	mux.mu.RLock()
	defer mux.mu.RUnlock()

	h, ok := mux.m[t]
	if !ok {
		return UnhandledHandler(t)
	}
	return h
}

// UnhandledHandler responds to the Request with a 300 invalid request error.
type UnhandledHandler string

// Parse returns a 300 invalid request error, without parsing anymore of
// the request.
func (t UnhandledHandler) Parse(b *bufio.Reader, req *Request) (*Request, error) {
	return req, UnknownRequestTypeError(string(t))
}

// Respond returns nothing since Parse always returns an error.
func (t UnhandledHandler) Respond(req *Request) (*Response, error) {
	return nil, nil
}

// atoi parses strings for ints. It's a simpler version of strconv.Atoi
// or strconv.ParseInt.
//
// This is shamelessly lifted from net.http.
func atoi(s string, i int) (n, i1 int, ok bool) {
	const Big = 1000000
	if i >= len(s) || s[i] < '0' || s[i] > '9' {
		return 0, 0, false
	}
	n = 0
	for ; i < len(s) && '0' <= s[i] && s[i] <= '9'; i++ {
		n = n*10 + int(s[i]-'0')
		if n > Big {
			return 0, 0, false
		}
	}
	return n, i, true
}

// parseGntpVersion extracts the two ints from a string like "GNTP/x.y".
//
// It returns 0, 0, false if there is any error in parsing.
func parseGntpVersion(version string) (major, minor int, ok bool) {
	if len(version) < 5 || version[0:5] != "GNTP/" {
		return 0, 0, false
	}
	major, i, ok := atoi(version, 5)
	if !ok || i >= len(version) || version[i] != '.' {
		return 0, 0, false
	}
	minor, i, ok = atoi(version, i+1)
	if !ok || i != len(version) {
		return 0, 0, false
	}
	return
}

// Parse reads the directive and first block of Header lines, then
// dispatches to the registered Handler's Parse function for the
// request's Type.
func (mux *ServeMux) Parse(b *bufio.Reader, req *Request) (*Request, error) {
	if req == nil {
		req = new(Request)
	}

	tp := textproto.NewReader(b)

	// Read the directive line.
	var s string
	var err error
	if s, err = tp.ReadLine(); err != nil {
		return req, err
	}

	// Split and parse the directive line.
	var f []string
	if f = strings.SplitN(s, " ", 3); len(f) < 3 {
		return req, UnknownProtocolError(s)
	}
	var ok bool
	if req.Version.Major, req.Version.Minor, ok = parseGntpVersion(f[0]); !ok {
		return req, UnknownProtocolError(s)
	}

	req.Type = f[1]

	// TODO: Handle security settings, if any.
	// For now we require NONE.
	if f[2] != "NONE" {
		return req, InvalidRequestError("unsupported encryption")
	}

	// Dispatch to the registered Handler's Parse function.
	return mux.handler(req.Type).Parse(b, req)
}

// Respond dispatches to the registered Handler's Respond function.
func (mux *ServeMux) Respond(req *Request) (*Response, error) {
	return mux.handler(req.Type).Respond(req)
}

// conn represents the connection between server and client.
type conn struct {
	remoteAddr string
	server     *Server
	rwc        net.Conn
	reader     *bufio.Reader
	writer     *bufio.Writer
}

// close flushes and closes a conn's writer and connection.
func (c *conn) close() {
	if c.writer != nil {
		c.writer.Flush()
		c.writer = nil
	}
	if c.rwc != nil {
		c.rwc.Close()
		c.rwc = nil
	}
}

// serve dispatches to the conn's Server's Handler's Parse and Respond
// functions. DefaultServeMux is used if the Handler is nil.
//
// Any panic's occuring in the Handler's functions are considered fatal
// to the conn, and result in the connection being closed without any
// further processing occuring.
func (c *conn) serve() {
	// Error (panic) recovery.
	defer func() {
		err := recover()
		if err == nil {
			return
		}

		var buf bytes.Buffer
		fmt.Fprintf(&buf, "gntp: panic serving %v: %v\n", c.remoteAddr, err)
		buf.Write(debug.Stack())
		log.Print(buf.String())

		if c.rwc != nil {
			c.rwc.Close()
		}
	}()

	// Close the conn when we're done.
	defer c.close()

	// Add (and later remove) ourself from the Server's WaitGroup.
	c.server.wg.Add(1)
	defer c.server.wg.Done()

	// Get the right Handler to use.
	handler := c.server.handler
	if handler == nil {
		handler = DefaultServeMux
	}

	var req *Request
	var resp *Response
	var err error
	// Dispatch to the Handler's Parse function.
	if req, err = handler.Parse(c.reader, req); err != nil {
		if ge, ok := err.(GntpError); ok {
			resp = ge.Response()
		} else {
			log.Println("gntp: could not parse request: " + err.Error())
			resp = InternalServerError().Response()
		}
	} else { // Successful parse
		if resp, err = handler.Respond(req); err != nil {
			if ge, ok := err.(*GntpError); ok {
				resp = ge.Response()
			} else {
				log.Println("gntp: could not create response: " + err.Error())
				resp = InternalServerError().Response()
			}
		}
	}

	// Write out our Response to the connection.
	resp.write(c.writer)
}

type Server struct {
	addr     string
	handler  Handler
	listener net.Listener
	shutdown bool
	wg       *sync.WaitGroup
}

// noLimit is effectively an infinite upper bound for io.LimitedReader.
const noLimit int64 = (1 << 63) - 1

// newConn builds a conn from a net.Conn for this Server.
func (srv *Server) newConn(rwc net.Conn) (c *conn) {
	c = new(conn)
	c.remoteAddr = rwc.RemoteAddr().String()
	c.server = srv
	c.rwc = rwc
	lr := io.LimitReader(rwc, noLimit).(*io.LimitedReader)
	c.reader = bufio.NewReader(lr)
	c.writer = bufio.NewWriter(rwc)
	return c
}

// New allocates and initializes a Server.
func New(addr string, handler Handler) *Server {
	return &Server{
		addr:    addr,
		handler: handler,
		wg:      new(sync.WaitGroup),
	}
}

// DefaultServer is the default Server used by Start() and Exit().
var DefaultServer = New("", nil)

// Start starts the DefaultServer.
func Start() error {
	return DefaultServer.Start()
}

// Start begins listening on the Server's address and handles each new
// connection in a seperate goroutine.
func (srv *Server) Start() error {
	addr := srv.addr
	if addr == "" {
		addr = ":gntp"
	}
	// "gntp" won't appear on most systems list of services, so use the
	// default port.
	if strings.HasSuffix(addr, ":gntp") {
		addr = addr[0:len(addr)-5] + ":23053"
	}

	var err error
	srv.listener, err = net.Listen("tcp", addr)
	if err != nil {
		return err
	}

	var tempDelay time.Duration
	for {
		rw, err := srv.listener.Accept()
		if err != nil {
			if nerr, ok := err.(net.Error); ok && nerr.Temporary() {
				// Account for temporary errors in accepting a connection, with
				// expontential backoff from 5ms upto 1s.
				if tempDelay == 0 {
					tempDelay = 5 * time.Millisecond
				} else {
					tempDelay *= 2
				}
				if max := 1 * time.Second; tempDelay > max {
					tempDelay = max
				}
				log.Printf("http: Accept error: %v, retrying in %v", err, tempDelay)
				time.Sleep(tempDelay)
				continue
			}
			if srv.shutdown {
				// Break out of the infinite loop if we've been asked to
				// shutdown.
				break
			}
			return err
		}
		tempDelay = 0

		// Handle each connection in a new goroutine.
		c := srv.newConn(rw)
		go c.serve()
	}

	// Wait for all goroutine'd connections to finish.
	srv.wg.Wait()
	return nil
}

// Exit exits the DefaultServer.
func Exit() {
	DefaultServer.Exit()
}

// Exit tells the Server to shutdown and closes it's listener.
//
// It doesn't allow any new connections to the server, but any existing
// connections will complete, if Start() was called first.
func (srv *Server) Exit() {
	log.Printf("debug: srv.Exit() called\n")
	srv.shutdown = true
	if srv.listener != nil {
		srv.listener.Close()
	}
}
