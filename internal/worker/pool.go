package worker

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/therealutkarshpriyadarshi/log/pkg/types"
)

var (
	ErrPoolClosed   = errors.New("worker pool is closed")
	ErrNoWorkers    = errors.New("no workers available")
	ErrJobTimeout   = errors.New("job execution timeout")
)

// JobFunc is a function that processes a log event
type JobFunc func(ctx context.Context, event *types.LogEvent) error

// PoolConfig holds configuration for the worker pool
type PoolConfig struct {
	NumWorkers   int
	QueueSize    int
	JobTimeout   time.Duration
	EnableStealing bool
}

// WorkerPool is a pool of workers that process log events
type WorkerPool struct {
	config   PoolConfig
	workers  []*worker
	jobQueue chan *job

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// Metrics
	jobsProcessed uint64
	jobsFailed    uint64
	jobsTimeout   uint64
	workersActive uint64
}

// worker represents a single worker in the pool
type worker struct {
	id         int
	pool       *WorkerPool
	jobQueue   chan *job
	jobFunc    JobFunc
	ctx        context.Context
	cancel     context.CancelFunc

	// Metrics
	jobsProcessed uint64
	jobsFailed    uint64
	lastActive    time.Time
	mu            sync.RWMutex
}

// job represents a unit of work
type job struct {
	event     *types.LogEvent
	resultCh  chan error
	createdAt time.Time
	timeout   time.Duration
}

// NewWorkerPool creates a new worker pool
func NewWorkerPool(config PoolConfig, jobFunc JobFunc) (*WorkerPool, error) {
	if config.NumWorkers <= 0 {
		config.NumWorkers = 4 // Default
	}

	if config.QueueSize <= 0 {
		config.QueueSize = 1000 // Default
	}

	if config.JobTimeout == 0 {
		config.JobTimeout = 30 * time.Second // Default
	}

	ctx, cancel := context.WithCancel(context.Background())

	pool := &WorkerPool{
		config:   config,
		workers:  make([]*worker, config.NumWorkers),
		jobQueue: make(chan *job, config.QueueSize),
		ctx:      ctx,
		cancel:   cancel,
	}

	// Create workers
	for i := 0; i < config.NumWorkers; i++ {
		pool.workers[i] = newWorker(i, pool, jobFunc)
	}

	return pool, nil
}

// Start starts all workers in the pool
func (p *WorkerPool) Start() {
	for _, w := range p.workers {
		p.wg.Add(1)
		go w.run()
	}
}

// Submit submits a job to the worker pool
func (p *WorkerPool) Submit(ctx context.Context, event *types.LogEvent) error {
	select {
	case <-p.ctx.Done():
		return ErrPoolClosed
	default:
	}

	j := &job{
		event:     event,
		resultCh:  make(chan error, 1),
		createdAt: time.Now(),
		timeout:   p.config.JobTimeout,
	}

	select {
	case p.jobQueue <- j:
		// Job submitted successfully
	case <-ctx.Done():
		return ctx.Err()
	case <-p.ctx.Done():
		return ErrPoolClosed
	}

	// Wait for result
	select {
	case err := <-j.resultCh:
		return err
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(j.timeout):
		atomic.AddUint64(&p.jobsTimeout, 1)
		return ErrJobTimeout
	}
}

// SubmitAsync submits a job without waiting for the result
func (p *WorkerPool) SubmitAsync(event *types.LogEvent) error {
	select {
	case <-p.ctx.Done():
		return ErrPoolClosed
	default:
	}

	j := &job{
		event:     event,
		resultCh:  make(chan error, 1),
		createdAt: time.Now(),
		timeout:   p.config.JobTimeout,
	}

	select {
	case p.jobQueue <- j:
		return nil
	case <-p.ctx.Done():
		return ErrPoolClosed
	default:
		return errors.New("job queue full")
	}
}

// Stop gracefully stops the worker pool
func (p *WorkerPool) Stop() error {
	p.cancel()

	// Close job queue
	close(p.jobQueue)

	// Wait for all workers to finish
	p.wg.Wait()

	return nil
}

// Scale adjusts the number of workers
func (p *WorkerPool) Scale(numWorkers int) error {
	if numWorkers <= 0 {
		return errors.New("number of workers must be positive")
	}

	select {
	case <-p.ctx.Done():
		return ErrPoolClosed
	default:
	}

	currentWorkers := len(p.workers)

	if numWorkers > currentWorkers {
		// Add more workers
		for i := currentWorkers; i < numWorkers; i++ {
			w := newWorker(i, p, p.workers[0].jobFunc)
			p.workers = append(p.workers, w)
			p.wg.Add(1)
			go w.run()
		}
	} else if numWorkers < currentWorkers {
		// Remove workers
		toRemove := currentWorkers - numWorkers
		for i := 0; i < toRemove; i++ {
			lastIdx := len(p.workers) - 1
			p.workers[lastIdx].stop()
			p.workers = p.workers[:lastIdx]
		}
	}

	p.config.NumWorkers = numWorkers
	return nil
}

// Metrics returns worker pool statistics
func (p *WorkerPool) Metrics() PoolMetrics {
	workerMetrics := make([]WorkerMetrics, len(p.workers))
	for i, w := range p.workers {
		workerMetrics[i] = w.metrics()
	}

	return PoolMetrics{
		NumWorkers:     len(p.workers),
		JobsProcessed:  atomic.LoadUint64(&p.jobsProcessed),
		JobsFailed:     atomic.LoadUint64(&p.jobsFailed),
		JobsTimeout:    atomic.LoadUint64(&p.jobsTimeout),
		WorkersActive:  atomic.LoadUint64(&p.workersActive),
		QueueSize:      len(p.jobQueue),
		QueueCapacity:  cap(p.jobQueue),
		WorkerMetrics:  workerMetrics,
	}
}

// newWorker creates a new worker
func newWorker(id int, pool *WorkerPool, jobFunc JobFunc) *worker {
	ctx, cancel := context.WithCancel(pool.ctx)

	return &worker{
		id:       id,
		pool:     pool,
		jobQueue: pool.jobQueue,
		jobFunc:  jobFunc,
		ctx:      ctx,
		cancel:   cancel,
	}
}

// run is the main worker loop
func (w *worker) run() {
	defer w.pool.wg.Done()

	for {
		select {
		case <-w.ctx.Done():
			return
		case j, ok := <-w.jobQueue:
			if !ok {
				return
			}

			w.processJob(j)
		}
	}
}

// processJob processes a single job
func (w *worker) processJob(j *job) {
	atomic.AddUint64(&w.pool.workersActive, 1)
	defer atomic.AddUint64(&w.pool.workersActive, ^uint64(0)) // Decrement

	w.mu.Lock()
	w.lastActive = time.Now()
	w.mu.Unlock()

	// Create timeout context
	ctx, cancel := context.WithTimeout(w.ctx, j.timeout)
	defer cancel()

	// Execute job
	err := w.jobFunc(ctx, j.event)

	atomic.AddUint64(&w.jobsProcessed, 1)
	atomic.AddUint64(&w.pool.jobsProcessed, 1)

	if err != nil {
		atomic.AddUint64(&w.jobsFailed, 1)
		atomic.AddUint64(&w.pool.jobsFailed, 1)
	}

	// Send result
	select {
	case j.resultCh <- err:
	default:
	}
}

// stop stops the worker
func (w *worker) stop() {
	w.cancel()
}

// metrics returns worker metrics
func (w *worker) metrics() WorkerMetrics {
	w.mu.RLock()
	defer w.mu.RUnlock()

	return WorkerMetrics{
		ID:            w.id,
		JobsProcessed: atomic.LoadUint64(&w.jobsProcessed),
		JobsFailed:    atomic.LoadUint64(&w.jobsFailed),
		LastActive:    w.lastActive,
	}
}

// stealJob attempts to steal a job from another worker (for work-stealing)
func (w *worker) stealJob() *job {
	if !w.pool.config.EnableStealing {
		return nil
	}

	// Try to steal from a random worker
	for _, other := range w.pool.workers {
		if other.id == w.id {
			continue
		}

		select {
		case j := <-other.jobQueue:
			return j
		default:
			continue
		}
	}

	return nil
}

// PoolMetrics holds worker pool statistics
type PoolMetrics struct {
	NumWorkers     int
	JobsProcessed  uint64
	JobsFailed     uint64
	JobsTimeout    uint64
	WorkersActive  uint64
	QueueSize      int
	QueueCapacity  int
	WorkerMetrics  []WorkerMetrics
}

// WorkerMetrics holds individual worker statistics
type WorkerMetrics struct {
	ID            int
	JobsProcessed uint64
	JobsFailed    uint64
	LastActive    time.Time
}

// Utilization returns the queue utilization percentage (0-100)
func (m PoolMetrics) Utilization() float64 {
	if m.QueueCapacity == 0 {
		return 0
	}
	return (float64(m.QueueSize) / float64(m.QueueCapacity)) * 100.0
}

// SuccessRate returns the job success rate percentage (0-100)
func (m PoolMetrics) SuccessRate() float64 {
	total := m.JobsProcessed
	if total == 0 {
		return 100.0
	}
	successful := total - m.JobsFailed
	return (float64(successful) / float64(total)) * 100.0
}
