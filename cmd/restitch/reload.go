// Copyright 2026 Restitch maintainers
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"
)

type reloader struct {
	configPath string
	mu         sync.Mutex
	reloadFn   func() (string, error)
}

func newReloader(configPath string, reloadFn func() (string, error)) *reloader {
	return &reloader{configPath: configPath, reloadFn: reloadFn}
}

func (r *reloader) reload() (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.reloadFn()
}

func (r *reloader) watchSignals() {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGHUP)
	go func() {
		for range ch {
			slog.Info("SIGHUP received, reloading config")
			if hash, err := r.reload(); err != nil {
				slog.Error("reload failed", "error", err)
			} else {
				slog.Info("config reloaded", "hash", hash)
			}
		}
	}()
}

func (r *reloader) watchFile() {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		slog.Warn("fsnotify unavailable", "error", err)
		return
	}

	dir := filepath.Dir(r.configPath)
	base := filepath.Base(r.configPath)
	if err := watcher.Add(dir); err != nil {
		slog.Warn("cannot watch config directory", "dir", dir, "error", err)
		watcher.Close()
		return
	}

	go func() {
		defer watcher.Close()
		var timer *time.Timer
		for {
			select {
			case ev, ok := <-watcher.Events:
				if !ok {
					return
				}
				if filepath.Base(ev.Name) != base {
					continue
				}
				if timer != nil {
					timer.Stop()
				}
				timer = time.AfterFunc(500*time.Millisecond, func() {
					slog.Info("config file changed, reloading")
					if hash, err := r.reload(); err != nil {
						slog.Error("reload failed", "error", err)
					} else {
						slog.Info("config reloaded", "hash", hash)
					}
				})
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				slog.Warn("fsnotify error", "error", err)
			}
		}
	}()
}
