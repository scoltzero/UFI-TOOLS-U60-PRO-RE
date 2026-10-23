// SPDX-License-Identifier: AGPL-3.0-or-later
package logfile

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"
)

// File bounds candidate diagnostics to two files. It never reads system logs.
type File struct {
	Path     string
	MaxBytes int64
	Enabled  func() bool
	mu       sync.Mutex
}

func (f *File) Write(p []byte) (int, error) {
	if f.Enabled != nil && !f.Enabled() {
		return len(p), nil
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	original := len(p)
	max := f.MaxBytes
	if max <= 0 {
		max = 1 << 20
	}
	if int64(len(p)) > max {
		p = p[len(p)-int(max):]
	}
	if err := os.MkdirAll(filepath.Dir(f.Path), 0700); err != nil {
		return 0, err
	}
	if info, err := os.Stat(f.Path); err == nil && info.Size()+int64(len(p)) > max {
		if err = os.Rename(f.Path, f.Path+".1"); err != nil {
			return 0, err
		}
	}
	out, err := os.OpenFile(f.Path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return 0, err
	}
	_, err = out.Write(p)
	err = errors.Join(err, out.Close())
	if err != nil {
		return 0, err
	}
	return original, nil
}
func (f *File) Read() ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	p, err := os.Open(f.Path)
	if errors.Is(err, os.ErrNotExist) {
		return []byte{}, nil
	}
	if err != nil {
		return nil, err
	}
	defer p.Close()
	return io.ReadAll(io.LimitReader(p, 1<<20))
}
func (f *File) Clear() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, name := range []string{f.Path, f.Path + ".1"} {
		if err := os.Remove(name); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}
