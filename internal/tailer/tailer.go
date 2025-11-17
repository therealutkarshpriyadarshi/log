package tailer

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"sync"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/therealutkarshpriyadarshi/log/internal/checkpoint"
	"github.com/therealutkarshpriyadarshi/log/internal/logging"
	"github.com/therealutkarshpriyadarshi/log/pkg/types"
)

// Tailer tails log files and handles rotation
type Tailer struct {
	paths          []string
	checkpointMgr  *checkpoint.Manager
	logger         *logging.Logger
	watcher        *fsnotify.Watcher
	files          map[string]*tailedFile
	mu             sync.RWMutex
	eventCh        chan *types.LogEvent
	ctx            context.Context
	cancel         context.CancelFunc
	wg             sync.WaitGroup
}

type tailedFile struct {
	path   string
	file   *os.File
	reader *bufio.Reader
	offset int64
	inode  uint64
}

// New creates a new Tailer instance
func New(paths []string, checkpointMgr *checkpoint.Manager, logger *logging.Logger) (*Tailer, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("failed to create file watcher: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	t := &Tailer{
		paths:         paths,
		checkpointMgr: checkpointMgr,
		logger:        logger.WithComponent("tailer"),
		watcher:       watcher,
		files:         make(map[string]*tailedFile),
		eventCh:       make(chan *types.LogEvent, 1000),
		ctx:           ctx,
		cancel:        cancel,
	}

	return t, nil
}

// Start starts tailing files
func (t *Tailer) Start() error {
	// Open all files
	for _, path := range t.paths {
		if err := t.openFile(path); err != nil {
			t.logger.Error().Err(err).Str("path", path).Msg("Failed to open file")
			// Continue with other files
		}
	}

	// Start watching for file events
	t.wg.Add(1)
	go t.watchLoop()

	return nil
}

// Stop stops the tailer
func (t *Tailer) Stop() {
	t.cancel()
	t.watcher.Close()
	t.wg.Wait()

	t.mu.Lock()
	defer t.mu.Unlock()

	for path, tf := range t.files {
		if tf.file != nil {
			t.checkpointMgr.UpdatePosition(path, tf.offset, tf.inode)
			tf.file.Close()
		}
	}

	close(t.eventCh)
}

// Events returns the channel for log events
func (t *Tailer) Events() <-chan *types.LogEvent {
	return t.eventCh
}

// openFile opens a file and starts tailing from the last checkpoint
func (t *Tailer) openFile(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}

	// Get file inode
	stat, err := file.Stat()
	if err != nil {
		file.Close()
		return fmt.Errorf("failed to stat file: %w", err)
	}

	inode := getInode(stat)

	// Check for checkpoint
	var offset int64
	if pos, ok := t.checkpointMgr.GetPosition(path); ok && pos.Inode == inode {
		offset = pos.Offset
		t.logger.Info().Str("path", path).Int64("offset", offset).Msg("Resuming from checkpoint")
	} else {
		// Start from end of file for new files
		offset, err = file.Seek(0, io.SeekEnd)
		if err != nil {
			file.Close()
			return fmt.Errorf("failed to seek file: %w", err)
		}
		t.logger.Info().Str("path", path).Msg("Starting from end of file")
	}

	// Seek to offset
	if _, err := file.Seek(offset, io.SeekStart); err != nil {
		file.Close()
		return fmt.Errorf("failed to seek to offset: %w", err)
	}

	tf := &tailedFile{
		path:   path,
		file:   file,
		reader: bufio.NewReader(file),
		offset: offset,
		inode:  inode,
	}

	t.mu.Lock()
	t.files[path] = tf
	t.mu.Unlock()

	// Add to watcher
	if err := t.watcher.Add(path); err != nil {
		t.logger.Warn().Err(err).Str("path", path).Msg("Failed to add file to watcher")
	}

	// Start reading from this file
	t.wg.Add(1)
	go t.readLoop(tf)

	return nil
}

// reopenFile handles file rotation by reopening the file
func (t *Tailer) reopenFile(path string) error {
	t.mu.Lock()
	tf, ok := t.files[path]
	t.mu.Unlock()

	if ok && tf.file != nil {
		t.checkpointMgr.UpdatePosition(path, tf.offset, tf.inode)
		tf.file.Close()
	}

	// Wait a bit for the new file to be created
	time.Sleep(100 * time.Millisecond)

	return t.openFile(path)
}

// readLoop reads lines from a file
func (t *Tailer) readLoop(tf *tailedFile) {
	defer t.wg.Done()

	for {
		select {
		case <-t.ctx.Done():
			return
		default:
		}

		line, err := tf.reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				// Wait for more data
				time.Sleep(100 * time.Millisecond)
				continue
			}
			t.logger.Error().Err(err).Str("path", tf.path).Msg("Error reading file")
			return
		}

		// Update offset
		tf.offset += int64(len(line))

		// Create log event
		event := &types.LogEvent{
			Timestamp: time.Now(),
			Message:   line,
			Source:    tf.path,
		}

		// Send event
		select {
		case t.eventCh <- event:
		case <-t.ctx.Done():
			return
		}

		// Periodically update checkpoint
		if tf.offset%10000 == 0 {
			t.checkpointMgr.UpdatePosition(tf.path, tf.offset, tf.inode)
		}
	}
}

// watchLoop watches for file events
func (t *Tailer) watchLoop() {
	defer t.wg.Done()

	for {
		select {
		case event, ok := <-t.watcher.Events:
			if !ok {
				return
			}

			t.handleEvent(event)

		case err, ok := <-t.watcher.Errors:
			if !ok {
				return
			}
			t.logger.Error().Err(err).Msg("File watcher error")

		case <-t.ctx.Done():
			return
		}
	}
}

// handleEvent handles file system events
func (t *Tailer) handleEvent(event fsnotify.Event) {
	path := event.Name

	switch {
	case event.Op&fsnotify.Write == fsnotify.Write:
		// File was written to, readLoop will pick up the changes
		t.logger.Debug().Str("path", path).Msg("File write event")

	case event.Op&fsnotify.Remove == fsnotify.Remove,
		event.Op&fsnotify.Rename == fsnotify.Rename:
		// File was removed or renamed (rotation)
		t.logger.Info().Str("path", path).Msg("File rotation detected")
		if err := t.reopenFile(path); err != nil {
			t.logger.Error().Err(err).Str("path", path).Msg("Failed to reopen file")
		}

	case event.Op&fsnotify.Create == fsnotify.Create:
		// New file created
		t.logger.Info().Str("path", path).Msg("File created")
		if err := t.openFile(path); err != nil {
			t.logger.Error().Err(err).Str("path", path).Msg("Failed to open file")
		}
	}
}

// getInode extracts inode from FileInfo
func getInode(fi os.FileInfo) uint64 {
	if stat, ok := fi.Sys().(*syscall.Stat_t); ok {
		return stat.Ino
	}
	return 0
}
