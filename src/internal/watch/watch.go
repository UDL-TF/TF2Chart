package watch

import (
	"context"
	"errors"
	"log"
	"os"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/UDL-TF/TF2Chart/src/internal/config"
	"github.com/UDL-TF/TF2Chart/src/internal/merge"
)

// Manager wires filesystem events into merger runs.
type Manager struct {
	merger   *merge.Merger
	cfg      *config.WatcherConfig
	debounce time.Duration
}

// NewManager creates a watcher manager.
func NewManager(merger *merge.Merger, cfg *config.WatcherConfig) (*Manager, error) {
	if merger == nil {
		return nil, errors.New("merger is required")
	}
	if cfg == nil {
		cfg = &config.WatcherConfig{}
	}
	debounce := time.Second * time.Duration(max(cfg.DebounceSeconds, 1))
	return &Manager{merger: merger, cfg: cfg, debounce: debounce}, nil
}

// Run blocks until the context is cancelled and reacts to filesystem events.
func (m *Manager) Run(ctx context.Context) error {
	if err := m.merger.Run(ctx); err != nil {
		return err
	}

	mergeRequests := make(chan struct{}, 1)
	immediateRequests := make(chan struct{}, 1)
	go m.mergeLoop(ctx, mergeRequests, immediateRequests)

	var pollTicker *time.Ticker
	if m.cfg.PollIntervalSeconds > 0 {
		pollTicker = time.NewTicker(time.Duration(m.cfg.PollIntervalSeconds) * time.Second)
		defer pollTicker.Stop()
	}
	var pollChan <-chan time.Time
	if pollTicker != nil {
		pollChan = pollTicker.C
	}

	if len(m.cfg.WatchPaths) == 0 {
		// Poll-only mode.
		if pollChan == nil {
			fallback := time.NewTicker(m.debounce)
			defer fallback.Stop()
			pollChan = fallback.C
		}
		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-pollChan:
				m.requestImmediate(immediateRequests)
			}
		}
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer watcher.Close()

	// Add only the top-level watch paths (non-recursive)
	for _, path := range m.cfg.WatchPaths {
		if path == "" {
			continue
		}
		if err := os.MkdirAll(path, 0o755); err != nil {
			log.Printf("watch mkdir %s: %v", path, err)
			continue
		}
		if err := watcher.Add(path); err != nil {
			log.Printf("watch add %s: %v", path, err)
			continue
		}
		log.Printf("watching: %s", path)
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-pollChan:
			m.requestImmediate(immediateRequests)
		case evt := <-watcher.Events:
			if evt.Op&(fsnotify.Create|fsnotify.Write|fsnotify.Remove|fsnotify.Rename) != 0 {
				m.requestMerge(mergeRequests)
			}
		case err := <-watcher.Errors:
			log.Printf("watch error: %v", err)
		}
	}
}

func (m *Manager) mergeLoop(ctx context.Context, scheduled <-chan struct{}, immediate <-chan struct{}) {
	timer := time.NewTimer(time.Hour)
	timer.Stop()
	timerActive := false
	pending := false
	for {
		select {
		case <-ctx.Done():
			return
		case <-scheduled:
			if pending {
				continue
			}
			pending = true
			if timerActive {
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
			}
			timer.Reset(m.debounce)
			timerActive = true
		case <-immediate:
			if timerActive {
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				timerActive = false
			}
			m.runMerge(ctx)
			pending = false
		case <-timer.C:
			timerActive = false
			m.runMerge(ctx)
			pending = false
		}
	}
}

func (m *Manager) requestMerge(ch chan<- struct{}) {
	select {
	case ch <- struct{}{}:
	default:
	}
}

func (m *Manager) requestImmediate(ch chan<- struct{}) {
	select {
	case ch <- struct{}{}:
	default:
	}
}

func (m *Manager) runMerge(ctx context.Context) {
	if err := m.merger.Run(ctx); err != nil {
		log.Printf("merge error: %v", err)
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
