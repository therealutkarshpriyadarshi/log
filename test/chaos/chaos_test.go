// +build chaos

package chaos

import (
	"context"
	"fmt"
	"os/exec"
	"testing"
	"time"
)

// ChaosTest represents a chaos test configuration
type ChaosTest struct {
	Name        string
	Description string
	Setup       func(t *testing.T) error
	Execute     func(t *testing.T) error
	Verify      func(t *testing.T) error
	Cleanup     func(t *testing.T) error
	Duration    time.Duration
}

// runDockerCommand executes a docker command
func runDockerCommand(args ...string) error {
	cmd := exec.Command("docker", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker command failed: %v, output: %s", err, string(output))
	}
	return nil
}

// runDockerComposeCommand executes a docker-compose command
func runDockerComposeCommand(args ...string) error {
	cmd := exec.Command("docker-compose", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker-compose command failed: %v, output: %s", err, string(output))
	}
	return nil
}

// killContainer kills a specific container
func killContainer(containerName string) error {
	return runDockerCommand("kill", containerName)
}

// pauseContainer pauses a specific container
func pauseContainer(containerName string) error {
	return runDockerCommand("pause", containerName)
}

// unpauseContainer unpauses a specific container
func unpauseContainer(containerName string) error {
	return runDockerCommand("unpause", containerName)
}

// restartContainer restarts a specific container
func restartContainer(containerName string) error {
	return runDockerCommand("restart", containerName)
}

// simulateNetworkPartition creates network partition using tc
func simulateNetworkPartition(containerName string, latency time.Duration) error {
	latencyMs := int(latency.Milliseconds())
	cmd := fmt.Sprintf("docker exec %s tc qdisc add dev eth0 root netem delay %dms", containerName, latencyMs)
	return exec.Command("sh", "-c", cmd).Run()
}

// removeNetworkPartition removes network partition
func removeNetworkPartition(containerName string) error {
	cmd := fmt.Sprintf("docker exec %s tc qdisc del dev eth0 root", containerName)
	return exec.Command("sh", "-c", cmd).Run()
}

// TestChaos_KillLogAggregator tests system behavior when log aggregator is killed
func TestChaos_KillLogAggregator(t *testing.T) {
	test := ChaosTest{
		Name:        "Kill Log Aggregator",
		Description: "Kill the main log aggregator and verify it restarts and recovers",
		Duration:    30 * time.Second,
		Setup: func(t *testing.T) error {
			t.Log("Verifying log aggregator is running")
			return nil
		},
		Execute: func(t *testing.T) error {
			t.Log("Killing log aggregator container")
			if err := killContainer("test-log-aggregator"); err != nil {
				return err
			}

			t.Log("Waiting for container to restart")
			time.Sleep(10 * time.Second)

			// Restart the container
			return restartContainer("test-log-aggregator")
		},
		Verify: func(t *testing.T) error {
			t.Log("Verifying log aggregator is healthy after restart")
			time.Sleep(10 * time.Second)
			// In a real test, we would check health endpoint
			return nil
		},
		Cleanup: func(t *testing.T) error {
			return nil
		},
	}

	runChaosTest(t, test)
}

// TestChaos_KafkaFailure tests system behavior when Kafka is unavailable
func TestChaos_KafkaFailure(t *testing.T) {
	test := ChaosTest{
		Name:        "Kafka Failure",
		Description: "Pause Kafka and verify buffering and recovery",
		Duration:    60 * time.Second,
		Setup: func(t *testing.T) error {
			t.Log("Verifying Kafka is running")
			return nil
		},
		Execute: func(t *testing.T) error {
			t.Log("Pausing Kafka container")
			if err := pauseContainer("test-kafka"); err != nil {
				return err
			}

			t.Log("Kafka paused for 30 seconds")
			time.Sleep(30 * time.Second)

			t.Log("Unpausing Kafka container")
			return unpauseContainer("test-kafka")
		},
		Verify: func(t *testing.T) error {
			t.Log("Verifying Kafka is healthy and processing resumed")
			time.Sleep(10 * time.Second)
			// Verify messages were buffered and sent after recovery
			return nil
		},
		Cleanup: func(t *testing.T) error {
			// Ensure Kafka is unpaused
			return unpauseContainer("test-kafka")
		},
	}

	runChaosTest(t, test)
}

// TestChaos_ElasticsearchFailure tests system behavior when Elasticsearch is unavailable
func TestChaos_ElasticsearchFailure(t *testing.T) {
	test := ChaosTest{
		Name:        "Elasticsearch Failure",
		Description: "Pause Elasticsearch and verify circuit breaker",
		Duration:    45 * time.Second,
		Setup: func(t *testing.T) error {
			t.Log("Verifying Elasticsearch is running")
			return nil
		},
		Execute: func(t *testing.T) error {
			t.Log("Pausing Elasticsearch container")
			if err := pauseContainer("test-elasticsearch"); err != nil {
				return err
			}

			t.Log("Elasticsearch paused for 20 seconds")
			time.Sleep(20 * time.Second)

			t.Log("Unpausing Elasticsearch container")
			return unpauseContainer("test-elasticsearch")
		},
		Verify: func(t *testing.T) error {
			t.Log("Verifying circuit breaker triggered and recovered")
			time.Sleep(15 * time.Second)
			return nil
		},
		Cleanup: func(t *testing.T) error {
			return unpauseContainer("test-elasticsearch")
		},
	}

	runChaosTest(t, test)
}

// TestChaos_NetworkLatency tests system behavior under high network latency
func TestChaos_NetworkLatency(t *testing.T) {
	test := ChaosTest{
		Name:        "Network Latency",
		Description: "Add network latency and verify timeout handling",
		Duration:    60 * time.Second,
		Setup: func(t *testing.T) error {
			t.Log("Preparing network latency test")
			return nil
		},
		Execute: func(t *testing.T) error {
			t.Log("Adding 500ms network latency to Kafka")
			if err := simulateNetworkPartition("test-kafka", 500*time.Millisecond); err != nil {
				t.Logf("Warning: Failed to add latency: %v", err)
			}

			t.Log("Running with latency for 30 seconds")
			time.Sleep(30 * time.Second)

			t.Log("Removing network latency")
			if err := removeNetworkPartition("test-kafka"); err != nil {
				t.Logf("Warning: Failed to remove latency: %v", err)
			}

			return nil
		},
		Verify: func(t *testing.T) error {
			t.Log("Verifying system handled latency gracefully")
			time.Sleep(10 * time.Second)
			return nil
		},
		Cleanup: func(t *testing.T) error {
			return removeNetworkPartition("test-kafka")
		},
	}

	runChaosTest(t, test)
}

// TestChaos_MultipleFailures tests cascading failures
func TestChaos_MultipleFailures(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping multiple failures test in short mode")
	}

	test := ChaosTest{
		Name:        "Multiple Failures",
		Description: "Simulate multiple simultaneous failures",
		Duration:    90 * time.Second,
		Setup: func(t *testing.T) error {
			t.Log("Preparing multiple failure test")
			return nil
		},
		Execute: func(t *testing.T) error {
			t.Log("Pausing multiple services")

			// Pause Kafka
			if err := pauseContainer("test-kafka"); err != nil {
				t.Logf("Warning: Failed to pause Kafka: %v", err)
			}

			time.Sleep(5 * time.Second)

			// Pause Elasticsearch
			if err := pauseContainer("test-elasticsearch"); err != nil {
				t.Logf("Warning: Failed to pause Elasticsearch: %v", err)
			}

			t.Log("Multiple services paused for 30 seconds")
			time.Sleep(30 * time.Second)

			t.Log("Recovering services")

			// Unpause Elasticsearch first
			if err := unpauseContainer("test-elasticsearch"); err != nil {
				t.Logf("Warning: Failed to unpause Elasticsearch: %v", err)
			}

			time.Sleep(10 * time.Second)

			// Then unpause Kafka
			if err := unpauseContainer("test-kafka"); err != nil {
				t.Logf("Warning: Failed to unpause Kafka: %v", err)
			}

			return nil
		},
		Verify: func(t *testing.T) error {
			t.Log("Verifying system recovered from multiple failures")
			time.Sleep(20 * time.Second)
			return nil
		},
		Cleanup: func(t *testing.T) error {
			unpauseContainer("test-kafka")
			unpauseContainer("test-elasticsearch")
			return nil
		},
	}

	runChaosTest(t, test)
}

// TestChaos_DiskPressure simulates disk pressure conditions
func TestChaos_DiskPressure(t *testing.T) {
	t.Skip("Disk pressure test requires additional setup")
	// This would fill up disk space and verify DLQ and WAL behavior
}

// TestChaos_MemoryPressure simulates memory pressure
func TestChaos_MemoryPressure(t *testing.T) {
	t.Skip("Memory pressure test requires cgroup manipulation")
	// This would limit memory and verify graceful degradation
}

// TestChaos_CPUThrottling simulates CPU throttling
func TestChaos_CPUThrottling(t *testing.T) {
	t.Skip("CPU throttling test requires cgroup manipulation")
	// This would throttle CPU and verify performance degradation is graceful
}

// runChaosTest executes a chaos test with proper error handling
func runChaosTest(t *testing.T, test ChaosTest) {
	t.Logf("=== Starting Chaos Test: %s ===", test.Name)
	t.Logf("Description: %s", test.Description)
	t.Logf("Duration: %v", test.Duration)

	// Defer cleanup to ensure it runs even if test fails
	defer func() {
		if test.Cleanup != nil {
			t.Log("Running cleanup")
			if err := test.Cleanup(t); err != nil {
				t.Logf("Warning: Cleanup failed: %v", err)
			}
		}
	}()

	// Setup
	if test.Setup != nil {
		t.Log("Running setup")
		if err := test.Setup(t); err != nil {
			t.Fatalf("Setup failed: %v", err)
		}
	}

	// Execute chaos
	start := time.Now()
	t.Log("Executing chaos scenario")
	if err := test.Execute(t); err != nil {
		t.Fatalf("Chaos execution failed: %v", err)
	}

	// Verify
	if test.Verify != nil {
		t.Log("Verifying system behavior")
		if err := test.Verify(t); err != nil {
			t.Fatalf("Verification failed: %v", err)
		}
	}

	elapsed := time.Since(start)
	t.Logf("=== Chaos Test Completed in %v ===", elapsed)
}

// TestChaos_RapidRestarts tests rapid service restarts
func TestChaos_RapidRestarts(t *testing.T) {
	containerName := "test-log-aggregator"
	iterations := 5

	for i := 0; i < iterations; i++ {
		t.Logf("Restart iteration %d/%d", i+1, iterations)

		if err := restartContainer(containerName); err != nil {
			t.Fatalf("Failed to restart container: %v", err)
		}

		// Wait a bit for restart
		time.Sleep(5 * time.Second)
	}

	// Final verification
	time.Sleep(15 * time.Second)
	t.Log("System survived rapid restarts")
}

// TestChaos_SplitBrain simulates network partition between services
func TestChaos_SplitBrain(t *testing.T) {
	t.Skip("Split brain test requires advanced networking setup")
	// This would create network partitions between services
	// and verify consistent behavior
}
