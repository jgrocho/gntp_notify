package main

import (
	"sync"
)

// Application represents an application.
type Application struct {
	Name          string
	Icon          string
	Count         int
	Notifications map[string]*Notification
}

// Applications maps names to applications.
type Applications struct {
	mu sync.RWMutex
	m  map[string]*Application
}

// Add adds an application to the applications.
func (apps *Applications) Add(app *Application) {
	apps.mu.Lock()
	defer apps.mu.Unlock()
	apps.m[app.Name] = app
}

// Get gets the application from the applications.
func (apps *Applications) Get(name string) *Application {
	apps.mu.RLock()
	defer apps.mu.RUnlock()
	return apps.m[name]
}

// NewApplications allocates and initializes Applications.
func NewApplications() *Applications {
	return &Applications{m: make(map[string]*Application)}
}
