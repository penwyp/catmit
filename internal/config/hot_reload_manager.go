package config

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/penwyp/catmit/internal/errors"
)

// HotReloadManager wraps a config manager with hot reload capability
type HotReloadManager struct {
	baseManager Manager
	configPath  string
	watcher     *fsnotify.Watcher
	
	// Current config stored atomically
	currentConfig atomic.Value // stores *Config
	
	// Callbacks for config changes
	callbacks []func(*Config)
	callbacksMu sync.RWMutex
	
	// Control
	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}
	
	// Debouncing
	debounceTimer *time.Timer
	debounceMu    sync.Mutex
}

// NewHotReloadManager creates a new hot reload manager
func NewHotReloadManager(baseManager Manager, configPath string) (*HotReloadManager, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, errors.Wrap(errors.ErrTypeConfig, "failed to create file watcher", err)
	}
	
	ctx, cancel := context.WithCancel(context.Background())
	
	m := &HotReloadManager{
		baseManager: baseManager,
		configPath:  configPath,
		watcher:     watcher,
		ctx:         ctx,
		cancel:      cancel,
		done:        make(chan struct{}),
		callbacks:   make([]func(*Config), 0),
	}
	
	// Load initial config
	config, err := baseManager.Load()
	if err != nil {
		if !os.IsNotExist(err) {
			watcher.Close()
			return nil, errors.Wrap(errors.ErrTypeConfig, "failed to load initial config", err)
		}
		// Create default config if not exists
		if err := baseManager.CreateDefaultConfig(); err != nil {
			watcher.Close()
			return nil, errors.Wrap(errors.ErrTypeConfig, "failed to create default config", err)
		}
		config, err = baseManager.Load()
		if err != nil {
			watcher.Close()
			return nil, errors.Wrap(errors.ErrTypeConfig, "failed to load default config", err)
		}
	}
	m.currentConfig.Store(config)
	
	// Start watching
	if err := m.startWatching(); err != nil {
		watcher.Close()
		return nil, err
	}
	
	return m, nil
}

// Load returns the current config (from memory)
func (m *HotReloadManager) Load() (*Config, error) {
	config := m.currentConfig.Load()
	if config == nil {
		return nil, errors.New(errors.ErrTypeConfig, "no config loaded")
	}
	return config.(*Config), nil
}

// Save saves the config and updates the in-memory cache
func (m *HotReloadManager) Save(config *Config) error {
	// Save to disk
	if err := m.baseManager.Save(config); err != nil {
		return err
	}
	
	// Update in-memory cache
	m.currentConfig.Store(config)
	
	// Notify callbacks
	m.notifyCallbacks(config)
	
	return nil
}

// CreateDefaultConfig creates the default config
func (m *HotReloadManager) CreateDefaultConfig() error {
	if err := m.baseManager.CreateDefaultConfig(); err != nil {
		return err
	}
	
	// Reload after creation
	config, err := m.baseManager.Load()
	if err != nil {
		return errors.Wrap(errors.ErrTypeConfig, "failed to load created config", err)
	}
	
	m.currentConfig.Store(config)
	m.notifyCallbacks(config)
	
	return nil
}

// UpdateRemote updates a remote config
func (m *HotReloadManager) UpdateRemote(host string, config RemoteConfig) error {
	// Update on disk
	if err := m.baseManager.UpdateRemote(host, config); err != nil {
		return err
	}
	
	// Reload to update cache
	newConfig, err := m.baseManager.Load()
	if err != nil {
		return errors.Wrap(errors.ErrTypeConfig, "failed to reload after update", err)
	}
	
	m.currentConfig.Store(newConfig)
	m.notifyCallbacks(newConfig)
	
	return nil
}

// OnConfigChange registers a callback for config changes
func (m *HotReloadManager) OnConfigChange(callback func(*Config)) {
	m.callbacksMu.Lock()
	defer m.callbacksMu.Unlock()
	m.callbacks = append(m.callbacks, callback)
}

// Stop stops the hot reload manager
func (m *HotReloadManager) Stop() error {
	m.cancel()
	
	// Stop any pending debounce timer
	m.debounceMu.Lock()
	if m.debounceTimer != nil {
		m.debounceTimer.Stop()
	}
	m.debounceMu.Unlock()
	
	// Wait for watcher to stop
	<-m.done
	
	return m.watcher.Close()
}

// startWatching starts the file watcher
func (m *HotReloadManager) startWatching() error {
	// Get the directory to watch
	dir := filepath.Dir(m.configPath)
	
	// Create directory if it doesn't exist
	if err := os.MkdirAll(dir, 0755); err != nil {
		return errors.Wrap(errors.ErrTypeConfig, "failed to create config directory", err)
	}
	
	// Add directory to watcher
	if err := m.watcher.Add(dir); err != nil {
		return errors.Wrap(errors.ErrTypeConfig, "failed to watch config directory", err)
	}
	
	// Start the watcher goroutine
	go m.watchLoop()
	
	return nil
}

// watchLoop is the main loop for watching file changes
func (m *HotReloadManager) watchLoop() {
	defer close(m.done)
	
	for {
		select {
		case <-m.ctx.Done():
			return
			
		case event, ok := <-m.watcher.Events:
			if !ok {
				return
			}
			
			// Check if the event is for our config file
			if filepath.Clean(event.Name) != filepath.Clean(m.configPath) {
				continue
			}
			
			// Handle different event types
			switch {
			case event.Op&fsnotify.Write == fsnotify.Write:
				m.handleConfigChange("write")
			case event.Op&fsnotify.Create == fsnotify.Create:
				m.handleConfigChange("create")
			case event.Op&fsnotify.Remove == fsnotify.Remove:
				// Config was removed, might be part of atomic write
				// Wait a bit to see if it's recreated
				m.handleConfigChange("remove")
			}
			
		case err, ok := <-m.watcher.Errors:
			if !ok {
				return
			}
			// Log the error but continue watching
			log.Printf("config watcher error: %v", err)
		}
	}
}

// handleConfigChange handles a config file change with debouncing
func (m *HotReloadManager) handleConfigChange(eventType string) {
	m.debounceMu.Lock()
	defer m.debounceMu.Unlock()
	
	// Cancel any existing timer
	if m.debounceTimer != nil {
		m.debounceTimer.Stop()
	}
	
	// Set a new timer - wait 100ms for changes to settle
	m.debounceTimer = time.AfterFunc(100*time.Millisecond, func() {
		m.reloadConfig()
	})
}

// reloadConfig reloads the config from disk
func (m *HotReloadManager) reloadConfig() {
	// Try to load the new config
	newConfig, err := m.baseManager.Load()
	if err != nil {
		if os.IsNotExist(err) {
			// Config was deleted, keep using the current one
			log.Printf("config file removed, keeping current config")
			return
		}
		// Log error but keep current config
		log.Printf("failed to reload config: %v", err)
		return
	}
	
	// Update the current config
	m.currentConfig.Store(newConfig)
	
	// Notify callbacks
	m.notifyCallbacks(newConfig)
}

// notifyCallbacks notifies all registered callbacks
func (m *HotReloadManager) notifyCallbacks(config *Config) {
	m.callbacksMu.RLock()
	callbacks := make([]func(*Config), len(m.callbacks))
	copy(callbacks, m.callbacks)
	m.callbacksMu.RUnlock()
	
	for _, callback := range callbacks {
		// Call each callback in a goroutine to prevent blocking
		go func(cb func(*Config)) {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("config change callback panic: %v", r)
				}
			}()
			cb(config)
		}(callback)
	}
}