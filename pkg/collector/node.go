package collector

import (
	"context"
	"fmt"
	"strings"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/procfs"
	"github.com/prometheus/procfs/blockdevice"
	"go.uber.org/zap"
	"github.com/strettch/sc-metrics-agent/pkg/config"
)

// ignoredFSTypes lists filesystem types to exclude from metrics collection
var ignoredFSTypes = map[string]bool{
	"autofs": true, "binfmt_misc": true, "cgroup": true, "cgroup2": true,
	"configfs": true, "debugfs": true, "devpts": true, "devtmpfs": true,
	"efivarfs": true, "fusectl": true, "hugetlbfs": true, "mqueue": true,
	"nsfs": true, "overlay": true, "proc": true, "procfs": true,
	"pstore": true, "rpc_pipefs": true, "securityfs": true, "selinuxfs": true,
	"squashfs": true, "sysfs": true, "tmpfs": true, "tracefs": true,
	"nfs": true, "nfs4": true, "cifs": true, "smb": true,
}

// Collector defines the interface for metric collectors
type Collector interface {
	Collect(ctx context.Context) ([]*dto.MetricFamily, error)
}

// SystemCollector implements system metrics collection using Prometheus collectors and procfs
type SystemCollector struct {
	registry    *prometheus.Registry
	logger      *zap.Logger
	enabled     map[string]bool
	procFS      procfs.FS
	lastCollect time.Time
}

// NewSystemCollector creates a new system collector using Prometheus libraries
func NewSystemCollector(cfg config.CollectorConfig, logger *zap.Logger) (*SystemCollector, error) {
	registry := prometheus.NewRegistry()
	enabled := make(map[string]bool)

	// Initialize procfs
	procFS, err := procfs.NewDefaultFS()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize procfs: %w", err)
	}

	sc := &SystemCollector{
		registry: registry,
		logger:   logger,
		enabled:  enabled,
		procFS:   procFS,
	}

	// Go runtime and process metrics removed - not useful for VM monitoring
	// These only track the agent itself, not the VM performance

	// Add custom system metrics based on configuration
	if cfg.CPU {
		if err := sc.addCPUCollector(registry); err == nil {
			enabled["cpu"] = true
			logger.Info("Enabled CPU collector")
		} else {
			logger.Warn("Failed to enable CPU collector", zap.Error(err))
		}
	}

	if cfg.Memory {
		if err := sc.addMemoryCollector(registry); err == nil {
			enabled["memory"] = true
			logger.Info("Enabled memory collector")
		} else {
			logger.Warn("Failed to enable memory collector", zap.Error(err))
		}
	}

	if cfg.LoadAvg {
		if err := sc.addLoadAvgCollector(registry); err == nil {
			enabled["loadavg"] = true
			logger.Info("Enabled load average collector")
		} else {
			logger.Warn("Failed to enable load average collector", zap.Error(err))
		}
	}

	if cfg.DiskStats {
		if err := sc.addDiskStatsCollector(registry); err == nil {
			enabled["diskstats"] = true
			logger.Info("Enabled disk stats collector")
		} else {
			logger.Warn("Failed to enable disk stats collector", zap.Error(err))
		}
	}

	if cfg.NetDev {
		if err := sc.addNetworkCollector(registry); err == nil {
			enabled["network"] = true
			logger.Info("Enabled network collector")
		} else {
			logger.Warn("Failed to enable network collector", zap.Error(err))
		}
	}

	if cfg.Filesystem {
		if err := sc.addFilesystemCollector(registry); err == nil {
			enabled["filesystem"] = true
			logger.Info("Enabled filesystem collector")
		} else {
			logger.Warn("Failed to enable filesystem collector", zap.Error(err))
		}
	}

	if len(enabled) == 0 {
		return nil, fmt.Errorf("no collectors enabled")
	}

	logger.Info("SystemCollector initialized", 
		zap.Int("enabled_collectors", len(enabled)),
		zap.Any("collectors", enabled))

	return sc, nil
}

// addCPUCollector adds CPU metrics using procfs
func (sc *SystemCollector) addCPUCollector(registry *prometheus.Registry) error {
	cpuCollector := &cpuCollector{procFS: sc.procFS, logger: sc.logger}
	registry.MustRegister(cpuCollector)
	return nil
}

// addMemoryCollector adds memory metrics using procfs
func (sc *SystemCollector) addMemoryCollector(registry *prometheus.Registry) error {
	memoryCollector := &memoryCollector{procFS: sc.procFS, logger: sc.logger}
	registry.MustRegister(memoryCollector)
	return nil
}

// addLoadAvgCollector adds load average metrics using procfs
func (sc *SystemCollector) addLoadAvgCollector(registry *prometheus.Registry) error {
	loadAvgCollector := &loadAvgCollector{procFS: sc.procFS, logger: sc.logger}
	registry.MustRegister(loadAvgCollector)
	return nil
}

// addDiskStatsCollector adds disk statistics metrics using procfs
func (sc *SystemCollector) addDiskStatsCollector(registry *prometheus.Registry) error {
	diskStatsCollector := &diskStatsCollector{procFS: sc.procFS, logger: sc.logger}
	registry.MustRegister(diskStatsCollector)
	return nil
}

// addNetworkCollector adds network metrics using procfs
func (sc *SystemCollector) addNetworkCollector(registry *prometheus.Registry) error {
	networkCollector := &networkCollector{procFS: sc.procFS, logger: sc.logger}
	registry.MustRegister(networkCollector)
	return nil
}

// addFilesystemCollector adds filesystem metrics
func (sc *SystemCollector) addFilesystemCollector(registry *prometheus.Registry) error {
	filesystemCollector := &filesystemCollector{procFS: sc.procFS, logger: sc.logger}
	registry.MustRegister(filesystemCollector)
	return nil
}

// Collect gathers metrics from all enabled collectors
func (sc *SystemCollector) Collect(ctx context.Context) ([]*dto.MetricFamily, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	sc.logger.Debug("Starting metric collection")
	start := time.Now()

	// Gather metrics from the registry
	metricFamilies, err := sc.registry.Gather()
	if err != nil {
		sc.logger.Error("Failed to gather metrics from registry", zap.Error(err))
		return nil, fmt.Errorf("failed to gather metrics: %w", err)
	}

	sc.lastCollect = time.Now()
	collectDuration := time.Since(start)

	sc.logger.Debug("Collected metrics",
		zap.Int("metric_families", len(metricFamilies)),
		zap.Duration("duration", collectDuration),
		zap.Int("enabled_collectors", len(sc.enabled)))

	return metricFamilies, nil
}

// GetEnabledCollectors returns a map of enabled collector names
func (sc *SystemCollector) GetEnabledCollectors() map[string]bool {
	result := make(map[string]bool)
	for k, v := range sc.enabled {
		result[k] = v
	}
	return result
}

// Close performs cleanup for the collector
func (sc *SystemCollector) Close() error {
	sc.logger.Debug("Closing system collector")
	return nil
}

// Custom collector implementations using procfs

type cpuCollector struct {
	procFS procfs.FS
	logger *zap.Logger
	desc   *prometheus.Desc
}

func (c *cpuCollector) Describe(ch chan<- *prometheus.Desc) {
	c.desc = prometheus.NewDesc("node_cpu_seconds_total", "Seconds the CPUs spent in each mode.", []string{"mode"}, nil)
	ch <- c.desc
}

func (c *cpuCollector) Collect(ch chan<- prometheus.Metric) {
	stat, err := c.procFS.Stat()
	if err != nil {
		c.logger.Debug("Failed to get CPU stats", zap.Error(err))
		return
	}

	// Only emit aggregate CPU stats (first entry in stat.CPU is the sum of all cores)
	if len(stat.CPU) == 0 {
		c.logger.Debug("No CPU stats available")
		return
	}

	cpu := stat.CPU[0] // First entry is aggregate across all cores

	ch <- prometheus.MustNewConstMetric(c.desc, prometheus.CounterValue, cpu.User, "user")
	ch <- prometheus.MustNewConstMetric(c.desc, prometheus.CounterValue, cpu.Nice, "nice")
	ch <- prometheus.MustNewConstMetric(c.desc, prometheus.CounterValue, cpu.System, "system")
	ch <- prometheus.MustNewConstMetric(c.desc, prometheus.CounterValue, cpu.Idle, "idle")
	ch <- prometheus.MustNewConstMetric(c.desc, prometheus.CounterValue, cpu.Iowait, "iowait")
	ch <- prometheus.MustNewConstMetric(c.desc, prometheus.CounterValue, cpu.IRQ, "irq")
	ch <- prometheus.MustNewConstMetric(c.desc, prometheus.CounterValue, cpu.SoftIRQ, "softirq")
	ch <- prometheus.MustNewConstMetric(c.desc, prometheus.CounterValue, cpu.Steal, "steal")
}

type memoryCollector struct {
	procFS procfs.FS
	logger *zap.Logger
	descs  map[string]*prometheus.Desc
}

func (c *memoryCollector) Describe(ch chan<- *prometheus.Desc) {
	c.descs = map[string]*prometheus.Desc{
		"MemTotal":     prometheus.NewDesc("node_memory_MemTotal_bytes", "Memory information field MemTotal_bytes.", nil, nil),
		"MemFree":      prometheus.NewDesc("node_memory_MemFree_bytes", "Memory information field MemFree_bytes.", nil, nil),
		"MemAvailable": prometheus.NewDesc("node_memory_MemAvailable_bytes", "Memory information field MemAvailable_bytes.", nil, nil),
		"Buffers":      prometheus.NewDesc("node_memory_Buffers_bytes", "Memory information field Buffers_bytes.", nil, nil),
		"Cached":       prometheus.NewDesc("node_memory_Cached_bytes", "Memory information field Cached_bytes.", nil, nil),
		"SwapTotal":    prometheus.NewDesc("node_memory_SwapTotal_bytes", "Memory information field SwapTotal_bytes.", nil, nil),
		"SwapFree":     prometheus.NewDesc("node_memory_SwapFree_bytes", "Memory information field SwapFree_bytes.", nil, nil),
	}

	for _, desc := range c.descs {
		ch <- desc
	}
}

func (c *memoryCollector) Collect(ch chan<- prometheus.Metric) {
	meminfo, err := c.procFS.Meminfo()
	if err != nil {
		c.logger.Debug("Failed to get memory info", zap.Error(err))
		return
	}

	if meminfo.MemTotal != nil {
		ch <- prometheus.MustNewConstMetric(c.descs["MemTotal"], prometheus.GaugeValue, float64(*meminfo.MemTotal*1024))
	}
	if meminfo.MemFree != nil {
		ch <- prometheus.MustNewConstMetric(c.descs["MemFree"], prometheus.GaugeValue, float64(*meminfo.MemFree*1024))
	}
	if meminfo.MemAvailable != nil {
		ch <- prometheus.MustNewConstMetric(c.descs["MemAvailable"], prometheus.GaugeValue, float64(*meminfo.MemAvailable*1024))
	}
	if meminfo.Buffers != nil {
		ch <- prometheus.MustNewConstMetric(c.descs["Buffers"], prometheus.GaugeValue, float64(*meminfo.Buffers*1024))
	}
	if meminfo.Cached != nil {
		ch <- prometheus.MustNewConstMetric(c.descs["Cached"], prometheus.GaugeValue, float64(*meminfo.Cached*1024))
	}
	if meminfo.SwapTotal != nil {
		ch <- prometheus.MustNewConstMetric(c.descs["SwapTotal"], prometheus.GaugeValue, float64(*meminfo.SwapTotal*1024))
	}
	if meminfo.SwapFree != nil {
		ch <- prometheus.MustNewConstMetric(c.descs["SwapFree"], prometheus.GaugeValue, float64(*meminfo.SwapFree*1024))
	}
}

type loadAvgCollector struct {
	procFS procfs.FS
	logger *zap.Logger
	descs  map[string]*prometheus.Desc
}

func (c *loadAvgCollector) Describe(ch chan<- *prometheus.Desc) {
	c.descs = map[string]*prometheus.Desc{
		"load1":  prometheus.NewDesc("node_load1", "1m load average.", nil, nil),
		"load5":  prometheus.NewDesc("node_load5", "5m load average.", nil, nil),
		"load15": prometheus.NewDesc("node_load15", "15m load average.", nil, nil),
	}

	for _, desc := range c.descs {
		ch <- desc
	}
}

func (c *loadAvgCollector) Collect(ch chan<- prometheus.Metric) {
	loadavg, err := c.procFS.LoadAvg()
	if err != nil {
		c.logger.Debug("Failed to get load average", zap.Error(err))
		return
	}

	ch <- prometheus.MustNewConstMetric(c.descs["load1"], prometheus.GaugeValue, loadavg.Load1)
	ch <- prometheus.MustNewConstMetric(c.descs["load5"], prometheus.GaugeValue, loadavg.Load5)
	ch <- prometheus.MustNewConstMetric(c.descs["load15"], prometheus.GaugeValue, loadavg.Load15)
}

type diskStatsCollector struct {
	procFS procfs.FS
	logger *zap.Logger
	descs  map[string]*prometheus.Desc
}

func (c *diskStatsCollector) Describe(ch chan<- *prometheus.Desc) {
	c.descs = map[string]*prometheus.Desc{
		"reads":      prometheus.NewDesc("node_disk_reads_completed_total", "The total number of reads completed successfully.", []string{"device"}, nil),
		"writes":     prometheus.NewDesc("node_disk_writes_completed_total", "The total number of writes completed successfully.", []string{"device"}, nil),
		"read_bytes": prometheus.NewDesc("node_disk_read_bytes_total", "The total number of bytes read successfully.", []string{"device"}, nil),
		"write_bytes": prometheus.NewDesc("node_disk_written_bytes_total", "The total number of bytes written successfully.", []string{"device"}, nil),
	}

	for _, desc := range c.descs {
		ch <- desc
	}
}

func (c *diskStatsCollector) Collect(ch chan<- prometheus.Metric) {
	// Use blockdevice package to get disk stats
	blockFS, err := blockdevice.NewFS("/proc", "/sys")
	if err != nil {
		c.logger.Debug("Failed to initialize blockdevice FS", zap.Error(err))
		return
	}

	diskStats, err := blockFS.ProcDiskstats()
	if err != nil {
		c.logger.Debug("Failed to get disk stats", zap.Error(err))
		return
	}

	for _, stat := range diskStats {
		// Skip loop devices, ram disks, and other virtual devices
		if strings.HasPrefix(stat.DeviceName, "loop") ||
			strings.HasPrefix(stat.DeviceName, "ram") ||
			strings.HasPrefix(stat.DeviceName, "dm-") {
			continue
		}

		ch <- prometheus.MustNewConstMetric(c.descs["reads"], prometheus.CounterValue, float64(stat.ReadIOs), stat.DeviceName)
		ch <- prometheus.MustNewConstMetric(c.descs["writes"], prometheus.CounterValue, float64(stat.WriteIOs), stat.DeviceName)
		ch <- prometheus.MustNewConstMetric(c.descs["read_bytes"], prometheus.CounterValue, float64(stat.ReadSectors*512), stat.DeviceName)
		ch <- prometheus.MustNewConstMetric(c.descs["write_bytes"], prometheus.CounterValue, float64(stat.WriteSectors*512), stat.DeviceName)
	}
}

type networkCollector struct {
	procFS procfs.FS
	logger *zap.Logger
	descs  map[string]*prometheus.Desc
}

func (c *networkCollector) Describe(ch chan<- *prometheus.Desc) {
	c.descs = map[string]*prometheus.Desc{
		"receive_bytes":   prometheus.NewDesc("node_network_receive_bytes_total", "Network device statistic receive_bytes.", []string{"device"}, nil),
		"transmit_bytes":  prometheus.NewDesc("node_network_transmit_bytes_total", "Network device statistic transmit_bytes.", []string{"device"}, nil),
		"receive_packets": prometheus.NewDesc("node_network_receive_packets_total", "Network device statistic receive_packets.", []string{"device"}, nil),
		"transmit_packets": prometheus.NewDesc("node_network_transmit_packets_total", "Network device statistic transmit_packets.", []string{"device"}, nil),
	}

	for _, desc := range c.descs {
		ch <- desc
	}
}

func (c *networkCollector) Collect(ch chan<- prometheus.Metric) {
	netDev, err := c.procFS.NetDev()
	if err != nil {
		c.logger.Debug("Failed to get network stats", zap.Error(err))
		return
	}

	for _, dev := range netDev {
		if dev.Name == "lo" {
			continue // Skip loopback
		}

		ch <- prometheus.MustNewConstMetric(c.descs["receive_bytes"], prometheus.CounterValue, float64(dev.RxBytes), dev.Name)
		ch <- prometheus.MustNewConstMetric(c.descs["transmit_bytes"], prometheus.CounterValue, float64(dev.TxBytes), dev.Name)
		ch <- prometheus.MustNewConstMetric(c.descs["receive_packets"], prometheus.CounterValue, float64(dev.RxPackets), dev.Name)
		ch <- prometheus.MustNewConstMetric(c.descs["transmit_packets"], prometheus.CounterValue, float64(dev.TxPackets), dev.Name)
	}
}

type filesystemCollector struct {
	procFS procfs.FS
	logger *zap.Logger
	descs  map[string]*prometheus.Desc
}

func (c *filesystemCollector) Describe(ch chan<- *prometheus.Desc) {
	c.descs = map[string]*prometheus.Desc{
		"size":  prometheus.NewDesc("node_filesystem_size_bytes", "Filesystem size in bytes.", []string{"device", "fstype", "mountpoint"}, nil),
		"free":  prometheus.NewDesc("node_filesystem_free_bytes", "Filesystem free space in bytes.", []string{"device", "fstype", "mountpoint"}, nil),
		"avail": prometheus.NewDesc("node_filesystem_avail_bytes", "Filesystem space available to non-root users in bytes.", []string{"device", "fstype", "mountpoint"}, nil),
	}

	for _, desc := range c.descs {
		ch <- desc
	}
}

func (c *filesystemCollector) Collect(ch chan<- prometheus.Metric) {
	mounts, err := procfs.GetMounts()
	if err != nil {
		c.logger.Debug("Failed to get mount information", zap.Error(err))
		return
	}

	for _, mount := range mounts {
		if ignoredFSTypes[mount.FSType] {
			c.logger.Debug("Skipping ignored filesystem type",
				zap.String("fstype", mount.FSType),
				zap.String("mountpoint", mount.MountPoint))
			continue
		}

		if !strings.HasPrefix(mount.Source, "/dev/") {
			c.logger.Debug("Skipping non-device filesystem", zap.String("source", mount.Source))
			continue
		}

		var stat syscall.Statfs_t
		if err := syscall.Statfs(mount.MountPoint, &stat); err != nil {
			c.logger.Debug("Failed to get filesystem stats",
				zap.String("mountpoint", mount.MountPoint),
				zap.Error(err))
			continue
		}

		totalSize := float64(stat.Blocks * uint64(stat.Bsize))
		freeSize := float64(stat.Bfree * uint64(stat.Bsize))
		availSize := float64(stat.Bavail * uint64(stat.Bsize))

		ch <- prometheus.MustNewConstMetric(c.descs["size"], prometheus.GaugeValue, totalSize, mount.Source, mount.FSType, mount.MountPoint)
		ch <- prometheus.MustNewConstMetric(c.descs["free"], prometheus.GaugeValue, freeSize, mount.Source, mount.FSType, mount.MountPoint)
		ch <- prometheus.MustNewConstMetric(c.descs["avail"], prometheus.GaugeValue, availSize, mount.Source, mount.FSType, mount.MountPoint)
	}
}