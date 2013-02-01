package main

import (
	"io"
	"os"
	"path/filepath"
)

// FileCache implements server.Binaries by saving files to disk.
type FileCache struct {
	dir string
}

// NewFileCache allocates and initializes a FileCache by saving files to dir.
func NewFileCache(dir string) *FileCache {
	return &FileCache{dir}
}

// Add reads length bytes from r and saves them to disk at key, under
// FileCache.dir.
func (cache *FileCache) Add(key string, length int64, r io.Reader) error {
	data := make([]byte, length)
	_, err := io.ReadFull(r, data)
	if err != nil && (err == io.EOF || err == io.ErrUnexpectedEOF) {
		return io.ErrUnexpectedEOF
	}

	path := filepath.Join(cache.dir, key)
	if _, err := os.Stat(path); err == nil {
		return nil
	}

	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	if _, err := file.Write(data); err != nil {
		return err
	}

	return nil
}

// Get gets the bytes from the file at key, under FileCache.dir.
func (cache *FileCache) Get(key string) ([]byte, error) {
	path := filepath.Join(cache.dir, key)
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}

	data := make([]byte, info.Size())
	_, err = io.ReadFull(file, data)
	if err != nil {
		return nil, err
	}

	return data, nil
}

// GetFileName gets the absolute filename for the data under key, if it exists.
//
// If the file does not exist on disk, it returns the empty string.
func (cache *FileCache) GetFileName(key string) string {
	path := filepath.Join(cache.dir, key)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return ""
	}
	if abs, err := filepath.Abs(path); err == nil {
		return abs
	}
	return ""
}

// Exists checks if the key file exists on disk.
func (cache *FileCache) Exists(key string) bool {
	path := filepath.Join(cache.dir, key)
	_, err := os.Stat(path)
	return err == nil
}
