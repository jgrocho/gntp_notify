package main

import (
	"flag"
	"github.com/jgrocho/gntp_notify/server"
	"io/ioutil"
	"log"
	"os"
	"os/signal"
	"time"
)

var (
	help     = flag.Bool("help", false, "Displays this help")
	cachedir = flag.String("cachedir", "", "Set an alternate cache directory")
)

func getCacheDir() (cacheDir string, err error) {
	var baseDir string
	if baseDir = os.Getenv("XDG_CACHE_HOME"); baseDir == "" {
		if homeDir := os.Getenv("HOME"); homeDir == "" {
			baseDir = os.TempDir()
		} else {
			baseDir = homeDir + "/.cache"
		}
	}
	cacheDir = baseDir + "/gntp_notify"
	if *cachedir != "" {
		cacheDir = *cachedir
	}
	err = os.MkdirAll(cacheDir, 0755)
	return
}

func main() {
	flag.Parse()

	if *help {
		flag.Usage()
		return
	}

	cacheDir, err := getCacheDir()
	if err != nil {
		log.Fatalf("could not create cache directory: %s\n", cacheDir)
	}
	if testFile, err := ioutil.TempDir(cacheDir, "test"); err != nil {
		log.Fatalf("cache directoy '%s' not writable\n", cacheDir)
	} else {
		if err = os.Remove(testFile); err != nil {
			log.Printf("could not remove temporary file: %v\n")
		}
	}

	binaryCache := NewFileCache(cacheDir)

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	go func() {
		for sig := range c {
			log.Printf("caputred %v, exiting\n", sig)
			go func() {
				time.Sleep(15 * time.Second)
				log.Printf("clean exit timed out, forcing\n")
				os.Exit(1)
			}()
			server.Exit()
		}
	}()

	apps := NewApplications()
	notes := NotificationChannel(binaryCache)

	server.Register("REGISTER", &RegisterHandler{apps, binaryCache})
	server.Register("NOTIFY", &NotifyHandler{apps, notes, binaryCache})
	server.Start()
	log.Println("Ending")
}
