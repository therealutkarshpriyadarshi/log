package input

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/therealutkarshpriyadarshi/log/internal/logging"
	"github.com/therealutkarshpriyadarshi/log/pkg/types"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// KubernetesConfig holds configuration for Kubernetes input
type KubernetesConfig struct {
	// Kubeconfig path (empty for in-cluster config)
	Kubeconfig string
	// Namespace to watch (empty for all namespaces)
	Namespace string
	// Label selector for pods
	LabelSelector string
	// Field selector for pods
	FieldSelector string
	// Container name pattern (empty for all containers)
	ContainerPattern string
	// Follow logs (tail -f behavior)
	Follow bool
	// Include previous container logs (for restarted containers)
	IncludePrevious bool
	// Tail lines (number of lines to tail from end, 0 for all)
	TailLines int64
	// Enrich with pod metadata
	EnrichMetadata bool
	// Buffer size for events channel
	BufferSize int
}

// KubernetesInput collects logs from Kubernetes pods
type KubernetesInput struct {
	*BaseInput
	config    *KubernetesConfig
	logger    *logging.Logger
	clientset *kubernetes.Clientset
	watcher   watch.Interface
	pods      map[string]*podInfo
	mu        sync.RWMutex
	wg        sync.WaitGroup
}

// podInfo tracks information about a pod
type podInfo struct {
	name       string
	namespace  string
	labels     map[string]string
	annotations map[string]string
	containers []string
	cancelFuncs []context.CancelFunc
}

// NewKubernetesInput creates a new Kubernetes input
func NewKubernetesInput(name string, config *KubernetesConfig, logger *logging.Logger) (*KubernetesInput, error) {
	if config.BufferSize == 0 {
		config.BufferSize = 10000
	}
	if config.Follow {
		// Default to following logs
		config.Follow = true
	}

	// Create Kubernetes client
	var kubeConfig *rest.Config
	var err error

	if config.Kubeconfig != "" {
		// Use kubeconfig file
		kubeConfig, err = clientcmd.BuildConfigFromFlags("", config.Kubeconfig)
	} else {
		// Use in-cluster config
		kubeConfig, err = rest.InClusterConfig()
	}

	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes config: %w", err)
	}

	clientset, err := kubernetes.NewForConfig(kubeConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes clientset: %w", err)
	}

	return &KubernetesInput{
		BaseInput: NewBaseInput(name, "kubernetes", config.BufferSize),
		config:    config,
		logger:    logger.WithComponent("input-kubernetes"),
		clientset: clientset,
		pods:      make(map[string]*podInfo),
	}, nil
}

// Start starts the Kubernetes log collector
func (k *KubernetesInput) Start() error {
	k.logger.Info().
		Str("namespace", k.config.Namespace).
		Str("label_selector", k.config.LabelSelector).
		Msg("Kubernetes log collector starting")

	// Start watching for pods
	if err := k.startWatcher(); err != nil {
		return fmt.Errorf("failed to start pod watcher: %w", err)
	}

	// Start collecting logs from existing pods
	if err := k.collectExistingPods(); err != nil {
		k.logger.Warn().Err(err).Msg("Failed to collect existing pods")
	}

	// Start watch loop
	k.wg.Add(1)
	go k.watchLoop()

	return nil
}

// Stop stops the Kubernetes log collector
func (k *KubernetesInput) Stop() error {
	k.logger.Info().Msg("Stopping Kubernetes log collector")

	k.Cancel()

	// Stop all pod log streams
	k.mu.Lock()
	for _, pod := range k.pods {
		for _, cancel := range pod.cancelFuncs {
			cancel()
		}
	}
	k.mu.Unlock()

	// Stop watcher
	if k.watcher != nil {
		k.watcher.Stop()
	}

	k.wg.Wait()
	k.Close()

	return nil
}

// Health returns the health status
func (k *KubernetesInput) Health() Health {
	k.mu.RLock()
	podCount := len(k.pods)
	k.mu.RUnlock()

	details := make(map[string]interface{})
	details["namespace"] = k.config.Namespace
	details["pods_watching"] = podCount

	return Health{
		Status:  HealthStatusHealthy,
		Message: "Kubernetes log collector is running",
		Details: details,
	}
}

// startWatcher starts watching for pod events
func (k *KubernetesInput) startWatcher() error {
	namespace := k.config.Namespace
	if namespace == "" {
		namespace = corev1.NamespaceAll
	}

	watcher, err := k.clientset.CoreV1().Pods(namespace).Watch(context.Background(), metav1.ListOptions{
		LabelSelector: k.config.LabelSelector,
		FieldSelector: k.config.FieldSelector,
	})

	if err != nil {
		return fmt.Errorf("failed to watch pods: %w", err)
	}

	k.watcher = watcher
	return nil
}

// collectExistingPods collects logs from existing pods
func (k *KubernetesInput) collectExistingPods() error {
	namespace := k.config.Namespace
	if namespace == "" {
		namespace = corev1.NamespaceAll
	}

	pods, err := k.clientset.CoreV1().Pods(namespace).List(context.Background(), metav1.ListOptions{
		LabelSelector: k.config.LabelSelector,
		FieldSelector: k.config.FieldSelector,
	})

	if err != nil {
		return fmt.Errorf("failed to list pods: %w", err)
	}

	for _, pod := range pods.Items {
		if pod.Status.Phase == corev1.PodRunning {
			k.handlePodAdded(&pod)
		}
	}

	k.logger.Info().Int("count", len(pods.Items)).Msg("Collected existing pods")
	return nil
}

// watchLoop watches for pod events
func (k *KubernetesInput) watchLoop() {
	defer k.wg.Done()

	for {
		select {
		case event, ok := <-k.watcher.ResultChan():
			if !ok {
				k.logger.Info().Msg("Pod watcher closed, restarting...")
				// Restart watcher
				if err := k.startWatcher(); err != nil {
					k.logger.Error().Err(err).Msg("Failed to restart watcher")
					time.Sleep(5 * time.Second)
				}
				continue
			}

			pod, ok := event.Object.(*corev1.Pod)
			if !ok {
				continue
			}

			switch event.Type {
			case watch.Added, watch.Modified:
				if pod.Status.Phase == corev1.PodRunning {
					k.handlePodAdded(pod)
				}
			case watch.Deleted:
				k.handlePodDeleted(pod)
			}

		case <-k.Context().Done():
			return
		}
	}
}

// handlePodAdded handles when a pod is added or becomes running
func (k *KubernetesInput) handlePodAdded(pod *corev1.Pod) {
	podKey := fmt.Sprintf("%s/%s", pod.Namespace, pod.Name)

	k.mu.RLock()
	_, exists := k.pods[podKey]
	k.mu.RUnlock()

	if exists {
		return // Already watching this pod
	}

	k.logger.Info().
		Str("namespace", pod.Namespace).
		Str("pod", pod.Name).
		Msg("Starting to collect logs from pod")

	info := &podInfo{
		name:        pod.Name,
		namespace:   pod.Namespace,
		labels:      pod.Labels,
		annotations: pod.Annotations,
		containers:  make([]string, 0),
		cancelFuncs: make([]context.CancelFunc, 0),
	}

	// Collect logs from all containers in the pod
	for _, container := range pod.Spec.Containers {
		// Check container pattern filter
		if k.config.ContainerPattern != "" {
			if !strings.Contains(container.Name, k.config.ContainerPattern) {
				continue
			}
		}

		info.containers = append(info.containers, container.Name)

		ctx, cancel := context.WithCancel(k.Context())
		info.cancelFuncs = append(info.cancelFuncs, cancel)

		k.wg.Add(1)
		go k.tailContainer(ctx, info, container.Name)
	}

	k.mu.Lock()
	k.pods[podKey] = info
	k.mu.Unlock()
}

// handlePodDeleted handles when a pod is deleted
func (k *KubernetesInput) handlePodDeleted(pod *corev1.Pod) {
	podKey := fmt.Sprintf("%s/%s", pod.Namespace, pod.Name)

	k.mu.Lock()
	info, exists := k.pods[podKey]
	if exists {
		for _, cancel := range info.cancelFuncs {
			cancel()
		}
		delete(k.pods, podKey)
	}
	k.mu.Unlock()

	if exists {
		k.logger.Info().
			Str("namespace", pod.Namespace).
			Str("pod", pod.Name).
			Msg("Stopped collecting logs from pod")
	}
}

// tailContainer tails logs from a container
func (k *KubernetesInput) tailContainer(ctx context.Context, pod *podInfo, containerName string) {
	defer k.wg.Done()

	k.logger.Debug().
		Str("namespace", pod.namespace).
		Str("pod", pod.name).
		Str("container", containerName).
		Msg("Tailing container logs")

	opts := &corev1.PodLogOptions{
		Container:  containerName,
		Follow:     k.config.Follow,
		Timestamps: true,
	}

	if k.config.TailLines > 0 {
		opts.TailLines = &k.config.TailLines
	}

	if k.config.IncludePrevious {
		opts.Previous = true
	}

	// Get log stream
	req := k.clientset.CoreV1().Pods(pod.namespace).GetLogs(pod.name, opts)
	stream, err := req.Stream(ctx)
	if err != nil {
		k.logger.Error().
			Err(err).
			Str("namespace", pod.namespace).
			Str("pod", pod.name).
			Str("container", containerName).
			Msg("Failed to get log stream")
		return
	}
	defer stream.Close()

	// Read logs line by line
	scanner := bufio.NewScanner(stream)
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return
		default:
		}

		line := scanner.Text()
		event := k.createEvent(line, pod, containerName)
		k.SendEvent(event)
	}

	if err := scanner.Err(); err != nil && err != io.EOF {
		k.logger.Error().
			Err(err).
			Str("namespace", pod.namespace).
			Str("pod", pod.name).
			Str("container", containerName).
			Msg("Error reading container logs")
	}
}

// createEvent creates a log event from a container log line
func (k *KubernetesInput) createEvent(line string, pod *podInfo, containerName string) *types.LogEvent {
	event := &types.LogEvent{
		Timestamp: time.Now(),
		Message:   line,
		Source:    k.name,
		Fields:    make(map[string]interface{}),
		Raw:       line,
	}

	// Add Kubernetes metadata if enabled
	if k.config.EnrichMetadata {
		event.Fields["kubernetes"] = map[string]interface{}{
			"namespace":   pod.namespace,
			"pod":         pod.name,
			"container":   containerName,
			"labels":      pod.labels,
			"annotations": pod.annotations,
		}
	} else {
		// Add minimal metadata
		event.Fields["namespace"] = pod.namespace
		event.Fields["pod"] = pod.name
		event.Fields["container"] = containerName
	}

	event.Fields["input_type"] = "kubernetes"

	return event
}
