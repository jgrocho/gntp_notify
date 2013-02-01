package main

import (
	"bufio"
	"crypto/md5"
	"fmt"
	"github.com/jgrocho/gntp_notify/server"
	"io"
	"log"
	"net/http"
	"net/textproto"
	"strconv"
	"strings"
)

// RegisterHandler handles GNTP REGISTER requests.
type RegisterHandler struct {
	apps        *Applications
	binaryCache *FileCache
}

// Parse parses GNTP REGISTER requests. It reads the Application block, each
// Notification block and any binary data sections.
func (handler *RegisterHandler) Parse(b *bufio.Reader, req *server.Request) (*server.Request, error) {
	tp := textproto.NewReader(b)

	h, err := tp.ReadMIMEHeader()
	if err != nil {
		return nil, err
	}
	header := server.Header(h)

	// Unfortunately, we have to repeat this parsing later. I have yet to find a
	// good way of passing the SAME arbitrary data structure between Parse and
	// Respond functions.
	countHeader, ok := header.Get("Notifications-Count")
	if !ok {
		return nil, server.MissingHeaderError("Notifications-Count")
	}
	count, err := strconv.Atoi(countHeader)
	if err != nil {
		return nil, server.InvalidRequestError("nofication count format invalid")
	}

	req.Headers = make([]server.Header, count+1)
	req.Headers[0] = header
	// NB: Cafeful with off-by-one errors in this section.
	for i := 1; i < count+1; i++ {
		h, err := tp.ReadMIMEHeader()
		if err != nil {
			return nil, err
		}
		req.Headers[i] = server.Header(h)
	}

	req.Binaries, err = server.ReadBinaries(b, req.Headers, handler.binaryCache)
	if err != nil {
		return nil, err
	}

	log.Printf("gntp: parsed REGISTER request: %+v\n", req)

	return req, nil
}

// download downloads the given URL and adds it to cache.
func download(url string, cache *FileCache) {
	// We are naively assuming that a URL's content never changes, and so the URL
	// can be used to uniquely identify the content.
	// TODO: Update the cache structure to be able to use HTTP caching mechanisms.
	hash := md5.New()
	io.WriteString(hash, url)
	sum := fmt.Sprintf("%x", hash.Sum(nil))

	if cache.Exists(sum) {
		return
	}

	resp, err := http.Get(url)
	if err != nil {
		log.Printf("gntp: Could not download %v\n", url)
		return
	}
	defer resp.Body.Close()

	// TODO: Update the cache structure so we can insert a key prior to attaching
	// the data to that key. This would allow us to delay showing notifications
	// that are waiting for an icon to download. It would also mean we could
	// guard FileCache.Add and FileCache.Get with a mutex to make it more thread
	// safe.
	cache.Add(sum, resp.ContentLength, resp.Body)
}

// buildApplication builds an Application (and it's corresponding notification
// types) from Header blocks.
func buildApplication(headers []server.Header, cache *FileCache) (*Application, error) {
	app := new(Application)
	appHeader := headers[0]

	var ok bool
	if app.Name, ok = appHeader.Get("Application-Name"); !ok || app.Name == "" {
		return nil, server.MissingHeaderError("Application-Name")
	}

	// Already did half the validation in Parse(). We'll repeat it here for
	// completeness sake.
	if count, ok := appHeader.Get("Notifications-Count"); !ok || count == "" {
		return nil, server.MissingHeaderError("Notifications-Count")
	} else {
		var err error
		if app.Count, err = strconv.Atoi(count); err != nil || app.Count < 0 {
			return nil, server.InvalidRequestError("notification count must be a non-negative integer")
		}
	}

	app.Icon, _ = appHeader.Get("Application-Icon")
	if app.Icon != "" && !strings.HasPrefix(strings.ToLower(app.Icon), "x-growl-resource://") {
		// For any icon that's not a GNTP resource identifier, download it in a new
		// goroutine.
		go download(app.Icon, cache)
	}

	app.Notifications = make(map[string]*Notification, app.Count)
	// NB: Be careful of off-by-one errors here.
	for i := 1; i < app.Count+1; i++ {
		note := new(Notification)
		noteHeader := headers[i]

		note.App = app

		if note.Name, ok = noteHeader.Get("Notification-Name"); !ok || note.Name == "" {
			log.Printf("header = %+v\n", noteHeader)
			return nil, server.MissingHeaderError("Notification-Name")
		}

		// Don't allow duplicate notifications.
		if _, present := app.Notifications[note.Name]; present {
			return nil, server.InvalidRequestError("Duplicate notification registered: " + note.Name)
		}

		if note.Display, ok = noteHeader.Get("Notification-Display"); !ok || note.Display == "" {
			note.Display = note.Name
		}

		// Notifications are not enabled by default, strconv.ParseBool() return
		// false for non-boolean-like values.
		enabled, _ := noteHeader.Get("Notification-Enabled")
		note.Enabled, _ = strconv.ParseBool(enabled)

		// Default to the application's icon.
		note.Icon = app.Icon
		if icon, ok := noteHeader.Get("Notification-Icon"); ok {
			// Use the notification icon, only if it is defined.
			note.Icon = icon
			if note.Icon != "" && !strings.HasPrefix(strings.ToLower(note.Icon), "x-growl-resource://") {
				// Download the icon if it's not a GNTP resource identifier. We should
				// not move this outside the outer if block. We don't need to
				// re-download the icon if it's the same as app.Icon.
				go download(note.Icon, cache)
			}
		}

		app.Notifications[note.Name] = note
	}

	return app, nil
}

// Respond builds the Application (and Notification defaults) and builds the
// response.
func (handler *RegisterHandler) Respond(req *server.Request) (*server.Response, error) {
	resp := server.NewResponse(1, 0)

	// Require GNTP/1.0
	if req.Version.Major != 1 && req.Version.Minor != 0 {
		return nil, server.UnknownProtocolVersionError(req.Version)
	}

	app, err := buildApplication(req.Headers, handler.binaryCache)
	if err != nil {
		return nil, err
	}
	handler.apps.Add(app)

	// Construct a simple Response.
	resp.Headers[0].Set("Response-Action", "REGISTER")

	return resp, nil
}

// NotifyHandler handles GNTP NOTIFY requests.
type NotifyHandler struct {
	apps        *Applications
	notes       chan *Notification
	binaryCache *FileCache
}

// Parse parses GNTP NOTIFY requests. It reads the Notification block and any
// binary data sections.
func (handler *NotifyHandler) Parse(b *bufio.Reader, req *server.Request) (*server.Request, error) {
	log.Println("gntp: NotifyHandler.Parse()")
	tp := textproto.NewReader(b)

	h, err := tp.ReadMIMEHeader()
	if err != nil {
		return nil, err
	}
	header := server.Header(h)
	req.Headers = make([]server.Header, 1)
	req.Headers[0] = header

	req.Binaries, err = server.ReadBinaries(b, req.Headers, handler.binaryCache)
	if err != nil {
		return nil, err
	}

	log.Printf("gntp: parsed NOTIFY request: %+v\n", req)

	return req, nil
}

// buildNotification builds a Notification from the Header block.
func buildNotification(apps *Applications, header server.Header, cache *FileCache) (*Notification, error) {
	note := new(Notification)

	appName, ok := header.Get("Application-Name")
	if !ok {
		return nil, server.MissingHeaderError("Application-Name")
	}
	app := apps.Get(appName)
	if app == nil {
		return nil, server.UnknownApplicationError(appName)
	}
	note.App = app

	if note.Name, ok = header.Get("Notification-Name"); !ok {
		return nil, server.MissingHeaderError("Notification-Name")
	}
	if note.Title, ok = header.Get("Notification-Title"); !ok {
		return nil, server.MissingHeaderError("Notification-Title")
	}

	// Get any defaults specified during registration.
	defaults, ok := app.Notifications[note.Name]
	if !ok {
		// The notification must be previously registered.
		return nil, server.UnknownNotificationError(appName, note.Name)
	}

	note.Enabled = defaults.Enabled

	note.Icon = defaults.Icon
	if icon, ok := header.Get("Notification-Icon"); ok {
		note.Icon = icon
		if note.Icon != "" && !strings.HasPrefix(strings.ToLower(note.Icon), "x-growl-resource://") {
			go download(note.Icon, cache)
		}
	}

	note.Id, _ = header.Get("Notification-Id")

	note.Text, _ = header.Get("Notification-Text")

	// Notifications are not sticky by default; strconv.ParseBool() returns false
	// for non-boolean-like values.
	sticky, _ := header.Get("Notification-Sticky")
	note.Sticky, _ = strconv.ParseBool(sticky)

	// Default priority is zero; strconv.Atoi() returns 0 for non-int-like
	// values.
	priority, _ := header.Get("Notification-Priority")
	note.Priority, _ = strconv.Atoi(priority)

	note.Coalescing, _ = header.Get("Notification-Coalescing")

	return note, nil
}

// Respond builds the Notification, sends it to be processed, and builds the
// reponse.
func (handler *NotifyHandler) Respond(req *server.Request) (*server.Response, error) {
	log.Println("gntp: NotifyHandler.Respond()")
	resp := server.NewResponse(1, 0)

	if req.Version.Major != 1 && req.Version.Minor != 0 {
		return nil, server.UnknownProtocolVersionError(req.Version)
	}

	note, err := buildNotification(handler.apps, req.Headers[0], handler.binaryCache)
	if err != nil {
		return nil, err
	}

	handler.notes <- note

	resp.Headers[0].Set("Response-Action", "NOTIFY")

	return resp, nil
}
