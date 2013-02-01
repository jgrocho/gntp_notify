package main

// #cgo pkg-config: libnotify
// #include <stdlib.h>
// #include <libnotify/notify.h>
import "C"
import (
	"crypto/md5"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"unsafe"
)

// Notification represents a notification.
type Notification struct {
	App        *Application
	Name       string
	Display    string
	Enabled    bool
	Icon       string
	Id         string
	Title      string
	Text       string
	Sticky     bool
	Priority   int
	Coalescing string
}

// NotifyUrgency represents the urgency of a notification for libnotify.
type NotifyUrgency int

// The recognized urgency levels of notifications.
const (
	NOTIFY_URGENCY_LOW NotifyUrgency = iota
	NOTIFY_URGENCY_NORMAL
	NOTIFY_URGENCY_CRITICAL
)

// NotifyTimeout represents the timeout of a notification for libnotify.
type NotifyTimeout int

// The recognized timeouts of notifications.
const (
	NOTIFY_EXPIRES_DEFAULT NotifyTimeout = iota - 1
	NOTIFY_EXPIRES_NEVER
)

// processNotification sends the notification to libnotify.
func processNotification(note *Notification, cache *FileCache) {
	if inited := bool(C.notify_is_initted() != 0); !inited {
		// We might be able to initialize libnotify here, if doing so is thread
		// safe and can be called multiple times.
		log.Println("gntp: libnotify is not initted")
		return
	}

	notify_title := C.CString(note.Title)
	defer C.free(unsafe.Pointer(notify_title))

	notify_text := C.CString(note.Text)
	defer C.free(unsafe.Pointer(notify_text))

	notify_icon := C.CString("")
	icon := note.Icon
	var iconFileName string
	if strings.HasPrefix(strings.ToLower(icon), "x-growl-resource://") {
		icon = icon[19:]
		iconFileName = cache.GetFileName(icon)
	} else if icon != "" {
		hash := md5.New()
		io.WriteString(hash, icon)
		sum := fmt.Sprintf("%x", hash.Sum(nil))
		iconFileName = cache.GetFileName(sum)
	}
	if _, err := os.Stat(iconFileName); err == nil {
		notify_icon = C.CString(iconFileName)
	}
	defer C.free(unsafe.Pointer(notify_icon))

	notify_notification := C.notify_notification_new(notify_title, notify_text, notify_icon)
	// TODO: Find the correct way to free notify_notification.
	//defer C.free(unsafe.Pointer(&notify_notification))

	notify_app_name := C.CString(note.App.Name)
	C.notify_notification_set_app_name(notify_notification, notify_app_name)
	defer C.free(unsafe.Pointer(notify_app_name))

	var urgency NotifyUrgency
	switch note.Priority {
	case -2, -1:
		urgency = NOTIFY_URGENCY_LOW
	case 0:
		urgency = NOTIFY_URGENCY_NORMAL
	case 1, 2:
		urgency = NOTIFY_URGENCY_CRITICAL
	default:
		log.Printf("gntp: unknown priority %v for notification %v from app %v\n", note.Priority, note.Name, note.App.Name)
		urgency = NOTIFY_URGENCY_NORMAL
	}
	notify_urgency := C.NotifyUrgency(urgency)
	C.notify_notification_set_urgency(notify_notification, notify_urgency)

	timeout := NOTIFY_EXPIRES_DEFAULT
	if note.Sticky {
		timeout = NOTIFY_EXPIRES_NEVER
	}
	notify_timeout := C.gint(timeout)
	C.notify_notification_set_timeout(notify_notification, notify_timeout)

	// Actually show the notification and report any error.
	var err *C.GError
	if shown := bool(C.notify_notification_show(notify_notification, &err) != 0); shown {
		log.Printf("Notification %s shown\n", note.Id)
	} else {
		log.Printf("Notification %s not shown\n", note.Id)
		if err != nil {
			message := C.GoString((*C.char)(err.message))
			log.Printf("  %s\n", message)
		}
	}
}

// NotificationChannel builds and returns a channel for Notifications.
func NotificationChannel(cache *FileCache) chan *Notification {
	c := make(chan *Notification)

	go func() {
		// libnotify needs a default app name when initialized. This will be
		// changed later.
		appName := C.CString("gntp_notify")
		defer C.free(unsafe.Pointer(appName))
		if inited := bool(C.notify_init(appName) != 0); !inited {
			log.Fatalf("gntp: Could not initialize libnotify")
		}
		defer C.notify_uninit()

		for {
			processNotification(<-c, cache)
		}
	}()

	return c
}
