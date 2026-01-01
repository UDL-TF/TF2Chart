package watch

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"syscall"
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
	log.Printf("watcher: running initial merge...")
	if err := m.merger.Run(ctx); err != nil {
		return fmt.Errorf("initial merge: %w", err)
	}
	log.Printf("watcher: initial merge completed successfully")

	// Debug: count open file descriptors
	if fds, err := countOpenFileDescriptors(); err == nil {
		log.Printf("watcher: open file descriptors after merge: %d", fds)
	}

	// Check inotify limits before creating watcher
	if err := checkInotifyLimits(); err != nil {
		log.Printf("watcher: inotify limit check: %v", err)
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
		log.Printf("watcher: FAILED to create fsnotify watcher: %v", err)
		log.Printf("watcher: falling back to poll-only mode (interval=%ds)", m.cfg.PollIntervalSeconds)

		// Fallback to polling mode when fsnotify fails
		if pollChan == nil {
			// Use debounce as poll interval if not configured
			pollInterval := time.Duration(m.cfg.PollIntervalSeconds) * time.Second
			if pollInterval == 0 {
				pollInterval = m.debounce * 2 // Default to 2x debounce time
			}
			pollTicker = time.NewTicker(pollInterval)
			defer pollTicker.Stop()
			pollChan = pollTicker.C
			log.Printf("watcher: configured polling every %s", pollInterval)
		}

		// Run in poll-only mode
		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-pollChan:
				m.requestImmediate(immediateRequests)
			}
		}
	}
	defer watcher.Close()

	log.Printf("watcher: created fsnotify watcher successfully")

	// Add only the top-level watch paths (non-recursive)
	for i, path := range m.cfg.WatchPaths {
		if path == "" {
			continue
		}
		if err := os.MkdirAll(path, 0o755); err != nil {
			log.Printf("watch mkdir %s: %v", path, err)
			continue
		}
		log.Printf("watcher: attempting to add watch %d/%d: %s", i+1, len(m.cfg.WatchPaths), path)
		if err := watcher.Add(path); err != nil {
			return fmt.Errorf("add watch for %s: %w", path, err)
		}
		log.Printf("watcher: successfully watching: %s", path)
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

func countOpenFileDescriptors() (int, error) {
	fds, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		// Fallback: try to get rlimit
		var rLimit syscall.Rlimit
		if err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &rLimit); err == nil {
			// Can't get exact count, return limit info as negative number to indicate estimation
			return -1, fmt.Errorf("cannot count (limit is %d)", rLimit.Cur)
		}
		return 0, err
	}
	return len(fds), nil
}

func checkInotifyLimits() error {
	// Try to read inotify limits
	maxInstances, err := os.ReadFile("/proc/sys/fs/inotify/max_user_instances")
	if err != nil {
		return fmt.Errorf("cannot read max_user_instances: %w", err)
	}

	maxWatches, err := os.ReadFile("/proc/sys/fs/inotify/max_user_watches")
	if err != nil {
		return fmt.Errorf("cannot read max_user_watches: %w", err)
	}

	log.Printf("inotify limits: max_user_instances=%s max_user_watches=%s",
		string(maxInstances[:len(maxInstances)-1]),
		string(maxWatches[:len(maxWatches)-1]))

	return nil
}
