package docker

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
	"time"
)

// Container represents a Docker container from the API
type Container struct {
	ID     string   `json:"Id"`
	Names  []string `json:"Names"`
	State  string   `json:"State"`
	Status string   `json:"Status"`
}

// Stats represents container statistics from Docker API
type Stats struct {
	Name    string `json:"name"`
	ID      string `json:"id"`
	Read    string `json:"read"`
	PreRead string `json:"preread"`

	PidsStats struct {
		Current uint64 `json:"current"`
		Limit   uint64 `json:"limit"`
	} `json:"pids_stats"`

	NumProcs     uint64                 `json:"num_procs"`
	StorageStats map[string]interface{} `json:"storage_stats"`

	CPUStats struct {
		CPUUsage struct {
			TotalUsage        uint64 `json:"total_usage"`
			UsageInKernelmode uint64 `json:"usage_in_kernelmode"`
			UsageInUsermode   uint64 `json:"usage_in_usermode"`
		} `json:"cpu_usage"`
		SystemCPUUsage uint64 `json:"system_cpu_usage"`
		OnlineCPUs     uint32 `json:"online_cpus"`
		ThrottlingData struct {
			Periods          uint64 `json:"periods"`
			ThrottledPeriods uint64 `json:"throttled_periods"`
			ThrottledTime    uint64 `json:"throttled_time"`
		} `json:"throttling_data"`
	} `json:"cpu_stats"`

	PreCPUStats struct {
		CPUUsage struct {
			TotalUsage        uint64 `json:"total_usage"`
			UsageInKernelmode uint64 `json:"usage_in_kernelmode"`
			UsageInUsermode   uint64 `json:"usage_in_usermode"`
		} `json:"cpu_usage"`
		SystemCPUUsage uint64 `json:"system_cpu_usage"`
		OnlineCPUs     uint32 `json:"online_cpus"`
		ThrottlingData struct {
			Periods          uint64 `json:"periods"`
			ThrottledPeriods uint64 `json:"throttled_periods"`
			ThrottledTime    uint64 `json:"throttled_time"`
		} `json:"throttling_data"`
	} `json:"precpu_stats"`

	MemoryStats struct {
		Usage uint64 `json:"usage"`
		Limit uint64 `json:"limit"`
		Stats struct {
			// Current field names used by Docker API
			ActiveAnon            uint64 `json:"active_anon"`
			ActiveFile            uint64 `json:"active_file"`
			Anon                  uint64 `json:"anon"`
			AnonThp               uint64 `json:"anon_thp"`
			File                  uint64 `json:"file"`
			FileDirty             uint64 `json:"file_dirty"`
			FileMapped            uint64 `json:"file_mapped"`
			FileWriteback         uint64 `json:"file_writeback"`
			InactiveAnon          uint64 `json:"inactive_anon"`
			InactiveFile          uint64 `json:"inactive_file"`
			KernelStack           uint64 `json:"kernel_stack"`
			Pgactivate            uint64 `json:"pgactivate"`
			Pgdeactivate          uint64 `json:"pgdeactivate"`
			Pgfault               uint64 `json:"pgfault"`
			Pglazyfree            uint64 `json:"pglazyfree"`
			Pglazyfreed           uint64 `json:"pglazyfreed"`
			Pgmajfault            uint64 `json:"pgmajfault"`
			Pgrefill              uint64 `json:"pgrefill"`
			Pgscan                uint64 `json:"pgscan"`
			Pgsteal               uint64 `json:"pgsteal"`
			Shmem                 uint64 `json:"shmem"`
			Slab                  uint64 `json:"slab"`
			SlabReclaimable       uint64 `json:"slab_reclaimable"`
			SlabUnreclaimable     uint64 `json:"slab_unreclaimable"`
			Sock                  uint64 `json:"sock"`
			ThpCollapseAlloc      uint64 `json:"thp_collapse_alloc"`
			ThpFaultAlloc         uint64 `json:"thp_fault_alloc"`
			Unevictable           uint64 `json:"unevictable"`
			WorkingsetActivate    uint64 `json:"workingset_activate"`
			WorkingsetNodereclaim uint64 `json:"workingset_nodereclaim"`
			WorkingsetRefault     uint64 `json:"workingset_refault"`
		} `json:"stats"`
	} `json:"memory_stats"`

	Networks map[string]struct {
		RxBytes   uint64 `json:"rx_bytes"`
		RxPackets uint64 `json:"rx_packets"`
		RxErrors  uint64 `json:"rx_errors"`
		RxDropped uint64 `json:"rx_dropped"`
		TxBytes   uint64 `json:"tx_bytes"`
		TxPackets uint64 `json:"tx_packets"`
		TxErrors  uint64 `json:"tx_errors"`
		TxDropped uint64 `json:"tx_dropped"`
	} `json:"networks"`

	// Only include the fields that actually have data (most are null)
	BlkioStats struct {
		IoServiceBytesRecursive []struct {
			Major uint64 `json:"major"`
			Minor uint64 `json:"minor"`
			Op    string `json:"op"`
			Value uint64 `json:"value"`
		} `json:"io_service_bytes_recursive"`
	} `json:"blkio_stats"`
}

// Client wraps HTTP client for Docker API communication
type Client struct {
	httpClient *http.Client
}

// NewClient creates a new Docker API client
func NewClient() *Client {
	return &Client{
		httpClient: &http.Client{
			Transport: &http.Transport{
				Dial: func(proto, addr string) (net.Conn, error) {
					return net.Dial("unix", "/var/run/docker.sock")
				},
			},
			Timeout: 30 * time.Second,
		},
	}
}

// ListContainers returns a list of all containers
func (c *Client) ListContainers() ([]Container, error) {
	resp, err := c.httpClient.Get("http://localhost/containers/json")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var containers []Container
	if err := json.NewDecoder(resp.Body).Decode(&containers); err != nil {
		return nil, err
	}

	return containers, nil
}

// GetContainerStats returns statistics for a specific container
func (c *Client) GetContainerStats(containerID string) (*Stats, error) {
	url := fmt.Sprintf("http://localhost/containers/%s/stats?stream=false", containerID)
	resp, err := c.httpClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var stats Stats
	if err := json.NewDecoder(resp.Body).Decode(&stats); err != nil {
		return nil, err
	}

	return &stats, nil
}

// CalculateCPUPercentage calculates CPU usage from container stats
func CalculateCPUPercentage(stats *Stats) float64 {
	// Try to use PreCPU stats for delta calculation
	cpuDelta := float64(stats.CPUStats.CPUUsage.TotalUsage) - float64(stats.PreCPUStats.CPUUsage.TotalUsage)
	systemDelta := float64(stats.CPUStats.SystemCPUUsage) - float64(stats.PreCPUStats.SystemCPUUsage)

	// If deltas are meaningful, calculate percentage
	if systemDelta > 0.0 && cpuDelta >= 0.0 {
		numCPUs := float64(stats.CPUStats.OnlineCPUs)
		if numCPUs == 0 {
			numCPUs = 1.0 // Fallback
		}
		cpuPercent := (cpuDelta / systemDelta) * numCPUs * 100.0
		return cpuPercent
	}

	return 0.0 // No meaningful CPU usage detected
}

// StatsCollector collects Docker container statistics and sends them via callback
func StatsCollector(dbAttributeName string, sleepTime int, recordEmptyOrZero bool, dataCallback func(string)) {
	log.Printf("Docker stats collector started (sleep: %ds)", sleepTime)
	client := NewClient()
	firstRun := true

	for {
		if !firstRun {
			time.Sleep(time.Duration(sleepTime) * time.Second)
		}
		firstRun = false

		// List all containers
		containers, err := client.ListContainers()
		if err != nil {
			log.Printf("[%s] Failed to list containers: %v", dbAttributeName, err)
			continue
		}

		// Get stats for each container
		for _, container := range containers {
			if container.State != "running" {
				continue // Skip stopped containers
			}

			log.Printf("TRACE: Processing container %s with ID %s", container.Names[0], container.ID)

			stats, err := client.GetContainerStats(container.ID)
			if err != nil {
				log.Printf("[%s] Failed to get stats for container %s: %v", dbAttributeName, container.Names[0], err)
				continue
			}

			// Container name (remove leading slash)
			containerName := strings.TrimPrefix(container.Names[0], "/")

			// Calculate CPU percentage
			cpuPercent := CalculateCPUPercentage(stats)

			// Calculate memory usage in MB (matching 'docker stats' behavior)
			// Working Set = Total Usage - Inactive File (reclaimable cache)
			totalUsage := stats.MemoryStats.Usage
			inactiveFile := stats.MemoryStats.Stats.InactiveFile
			workingSetUsage := totalUsage - inactiveFile

			memoryUsageMB := float64(workingSetUsage) / 1024 / 1024 // This now matches 'docker stats'
			memoryLimitMB := float64(stats.MemoryStats.Limit) / 1024 / 1024
			memoryPercent := 0.0
			if memoryLimitMB > 0 {
				memoryPercent = (memoryUsageMB / memoryLimitMB) * 100
			}

			// Calculate network I/O
			var networkRxBytes, networkTxBytes uint64
			for _, network := range stats.Networks {
				networkRxBytes += network.RxBytes
				networkTxBytes += network.TxBytes
			}

			// Calculate block I/O
			var blockRead, blockWrite uint64
			for _, bioEntry := range stats.BlkioStats.IoServiceBytesRecursive {
				if bioEntry.Op == "read" || bioEntry.Op == "Read" {
					blockRead += bioEntry.Value
				} else if bioEntry.Op == "write" || bioEntry.Op == "Write" {
					blockWrite += bioEntry.Value
				}
			}

			// Prepare InfluxDB payload
			payload := fmt.Sprintf("%s,container=%s cpu_percent=%f,memory_usage_mb=%f,memory_limit_mb=%f,memory_percent=%f,network_rx_bytes=%d,network_tx_bytes=%d,block_read_bytes=%d,block_write_bytes=%d",
				dbAttributeName,
				containerName,
				cpuPercent,
				memoryUsageMB,
				memoryLimitMB,
				memoryPercent,
				networkRxBytes,
				networkTxBytes,
				blockRead,
				blockWrite,
			)

			// Send data via callback
			dataCallback(payload)
		}
	}
}
