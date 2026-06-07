package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"sandboxd-o/pkg/envutil"
)

const (
	DefaultConfigPath            = "/var/lib/sandboxd/sbxorch_config.json"
	defaultHTTPAddr              = ":8082"
	defaultSQLitePath            = "/var/lib/sandboxd/orchestrator.db"
	defaultHeartbeatInterval     = 10 * time.Second
	defaultProbeTimeout          = 3 * time.Second
	defaultSandboxOpTimeout      = 60 * time.Second
	defaultHeartbeatParallel     = false
	defaultHeartbeatMaxParallel  = 4
	defaultResourceSyncInterval  = 30 * time.Second
	defaultResourcePersistMinInt = 30 * time.Second
	defaultResourcePersistMaxInt = 5 * time.Minute
	defaultReadySuccessThreshold = 2
	defaultNotReadyFailThreshold = 2
	defaultShutdownTimeout       = 5 * time.Second
	defaultSchedulerInterval     = 3 * time.Second
	defaultReconcileInterval     = 5 * time.Second
	defaultStatusSyncInterval    = 20 * time.Second
	defaultStatusSyncTimeout     = 5 * time.Second
	defaultStatusSyncBatchSize   = 50
	defaultStatusSyncMaxParallel = 4
	defaultPortMin               = 10000
	defaultPortMax               = 32767
	defaultCreateRPS             = 20.0
	defaultCreateBurst           = 40
	defaultLogFilePrefix         = "orchestrator"
)

type Config struct {
	AppEnv                   string        `json:"app_env,omitempty"`
	HTTPAddr                 string        `json:"http_addr,omitempty"`
	LogDir                   string        `json:"log_dir,omitempty"`
	LogFilePrefix            string        `json:"log_file_prefix,omitempty"`
	SQLitePath               string        `json:"sqlite_path,omitempty"`
	HeartbeatInterval        time.Duration `json:"heartbeat_interval,omitempty"`
	ProbeTimeout             time.Duration `json:"probe_timeout,omitempty"`
	SandboxOpTimeout         time.Duration `json:"sandbox_op_timeout,omitempty"`
	HeartbeatParallel        bool          `json:"heartbeat_parallel,omitempty"`
	HeartbeatMaxParallel     int           `json:"heartbeat_max_parallel,omitempty"`
	ResourceSyncInterval     time.Duration `json:"resource_sync_interval,omitempty"`
	ResourcePersistMinInt    time.Duration `json:"resource_persist_min_interval,omitempty"`
	ResourcePersistMaxInt    time.Duration `json:"resource_persist_max_interval,omitempty"`
	ReadySuccessThreshold    int           `json:"ready_success_threshold,omitempty"`
	NotReadyFailureThreshold int           `json:"notready_failure_threshold,omitempty"`
	ShutdownTimeout          time.Duration `json:"shutdown_timeout,omitempty"`
	SchedulerInterval        time.Duration `json:"scheduler_interval,omitempty"`
	ReconcileInterval        time.Duration `json:"reconcile_interval,omitempty"`
	StatusSyncInterval       time.Duration `json:"status_sync_interval,omitempty"`
	StatusSyncTimeout        time.Duration `json:"status_sync_timeout,omitempty"`
	StatusSyncBatchSize      int           `json:"status_sync_batch_size,omitempty"`
	StatusSyncMaxParallel    int           `json:"status_sync_max_parallel,omitempty"`
	HostPortMin              int           `json:"host_port_min,omitempty"`
	HostPortMax              int           `json:"host_port_max,omitempty"`
	CreateRPS                float64       `json:"create_rps,omitempty"`
	CreateBurst              int           `json:"create_burst,omitempty"`
}

func DefaultConfig() Config {
	return Config{
		HTTPAddr:                 defaultHTTPAddr,
		LogFilePrefix:            defaultLogFilePrefix,
		SQLitePath:               defaultSQLitePath,
		HeartbeatInterval:        defaultHeartbeatInterval,
		ProbeTimeout:             defaultProbeTimeout,
		SandboxOpTimeout:         defaultSandboxOpTimeout,
		HeartbeatParallel:        defaultHeartbeatParallel,
		HeartbeatMaxParallel:     defaultHeartbeatMaxParallel,
		ResourceSyncInterval:     defaultResourceSyncInterval,
		ResourcePersistMinInt:    defaultResourcePersistMinInt,
		ResourcePersistMaxInt:    defaultResourcePersistMaxInt,
		ReadySuccessThreshold:    defaultReadySuccessThreshold,
		NotReadyFailureThreshold: defaultNotReadyFailThreshold,
		ShutdownTimeout:          defaultShutdownTimeout,
		SchedulerInterval:        defaultSchedulerInterval,
		ReconcileInterval:        defaultReconcileInterval,
		StatusSyncInterval:       defaultStatusSyncInterval,
		StatusSyncTimeout:        defaultStatusSyncTimeout,
		StatusSyncBatchSize:      defaultStatusSyncBatchSize,
		StatusSyncMaxParallel:    defaultStatusSyncMaxParallel,
		HostPortMin:              defaultPortMin,
		HostPortMax:              defaultPortMax,
		CreateRPS:                defaultCreateRPS,
		CreateBurst:              defaultCreateBurst,
	}
}

func Load(path string) (Config, error) {
	cfg := DefaultConfig()
	if err := loadFile(path, &cfg); err != nil {
		return Config{}, err
	}
	applyEnvOverrides(&cfg)
	normalize(&cfg)

	if cfg.SQLitePath != ":memory:" {
		dir := filepath.Dir(cfg.SQLitePath)
		if dir != "" && dir != "." {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return Config{}, fmt.Errorf("create sqlite dir: %w", err)
			}
		}
	}

	return cfg, nil
}

func loadFile(path string, cfg *Config) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read config file %q: %w", path, err)
	}

	var fileCfg orchFileConfig
	if err := json.Unmarshal(raw, &fileCfg); err != nil {
		return fmt.Errorf("parse config file %q: %w", path, err)
	}
	fileCfg.apply(cfg)

	return nil
}

func applyEnvOverrides(cfg *Config) {
	cfg.AppEnv = envutil.Get("APP_ENV", cfg.AppEnv)
	cfg.HTTPAddr = envutil.Get("ORCH_HTTP_ADDR", cfg.HTTPAddr)
	cfg.LogDir = envutil.Get("ORCH_LOG_DIR", cfg.LogDir)
	cfg.LogFilePrefix = envutil.Get("ORCH_LOG_FILE_PREFIX", cfg.LogFilePrefix)
	cfg.SQLitePath = envutil.Get("ORCH_SQLITE_PATH", cfg.SQLitePath)
	cfg.HeartbeatInterval = envutil.GetDuration("ORCH_HEARTBEAT_INTERVAL", cfg.HeartbeatInterval)
	cfg.ProbeTimeout = envutil.GetDuration("ORCH_NODE_PROBE_TIMEOUT", cfg.ProbeTimeout)
	cfg.SandboxOpTimeout = envutil.GetDuration("ORCH_SANDBOX_OP_TIMEOUT", cfg.SandboxOpTimeout)
	cfg.HeartbeatParallel = envutil.GetBool("ORCH_HEARTBEAT_PARALLEL", cfg.HeartbeatParallel)
	cfg.HeartbeatMaxParallel = envutil.GetInt("ORCH_HEARTBEAT_MAX_PARALLEL", cfg.HeartbeatMaxParallel)
	cfg.ResourceSyncInterval = envutil.GetDuration("ORCH_RESOURCE_SYNC_INTERVAL", cfg.ResourceSyncInterval)
	cfg.ResourcePersistMinInt = envutil.GetDuration("ORCH_RESOURCE_PERSIST_MIN_INTERVAL", cfg.ResourcePersistMinInt)
	cfg.ResourcePersistMaxInt = envutil.GetDuration("ORCH_RESOURCE_PERSIST_MAX_INTERVAL", cfg.ResourcePersistMaxInt)
	cfg.ReadySuccessThreshold = envutil.GetInt("ORCH_READY_SUCCESS_THRESHOLD", cfg.ReadySuccessThreshold)
	cfg.NotReadyFailureThreshold = envutil.GetInt("ORCH_NOTREADY_FAILURE_THRESHOLD", cfg.NotReadyFailureThreshold)
	cfg.ShutdownTimeout = envutil.GetDuration("ORCH_SHUTDOWN_TIMEOUT", cfg.ShutdownTimeout)
	cfg.SchedulerInterval = envutil.GetDuration("ORCH_SCHEDULER_INTERVAL", cfg.SchedulerInterval)
	cfg.ReconcileInterval = envutil.GetDuration("ORCH_RECONCILE_INTERVAL", cfg.ReconcileInterval)
	cfg.StatusSyncInterval = envutil.GetDuration("ORCH_STATUS_SYNC_INTERVAL", cfg.StatusSyncInterval)
	cfg.StatusSyncTimeout = envutil.GetDuration("ORCH_STATUS_SYNC_TIMEOUT", cfg.StatusSyncTimeout)
	cfg.StatusSyncBatchSize = envutil.GetInt("ORCH_STATUS_SYNC_BATCH_SIZE", cfg.StatusSyncBatchSize)
	cfg.StatusSyncMaxParallel = envutil.GetInt("ORCH_STATUS_SYNC_MAX_PARALLEL", cfg.StatusSyncMaxParallel)
	cfg.HostPortMin = envutil.GetInt("ORCH_HOSTPORT_MIN", cfg.HostPortMin)
	cfg.HostPortMax = envutil.GetInt("ORCH_HOSTPORT_MAX", cfg.HostPortMax)
	cfg.CreateRPS = envutil.GetFloat64("ORCH_CREATE_RPS", cfg.CreateRPS)
	cfg.CreateBurst = envutil.GetInt("ORCH_CREATE_BURST", cfg.CreateBurst)
}

func normalize(cfg *Config) {
	if cfg.HTTPAddr == "" {
		cfg.HTTPAddr = defaultHTTPAddr
	}
	if cfg.LogFilePrefix == "" {
		cfg.LogFilePrefix = defaultLogFilePrefix
	}
	if cfg.SQLitePath == "" {
		cfg.SQLitePath = defaultSQLitePath
	}
	if cfg.ReadySuccessThreshold < 1 {
		cfg.ReadySuccessThreshold = 1
	}
	if cfg.NotReadyFailureThreshold < 1 {
		cfg.NotReadyFailureThreshold = 1
	}
	if cfg.HeartbeatMaxParallel < 1 {
		cfg.HeartbeatMaxParallel = 1
	}
	if cfg.ResourceSyncInterval <= 0 {
		cfg.ResourceSyncInterval = defaultResourceSyncInterval
	}
	if cfg.ResourcePersistMinInt <= 0 {
		cfg.ResourcePersistMinInt = defaultResourcePersistMinInt
	}
	if cfg.ResourcePersistMaxInt <= 0 {
		cfg.ResourcePersistMaxInt = defaultResourcePersistMaxInt
	}
	if cfg.SchedulerInterval <= 0 {
		cfg.SchedulerInterval = defaultSchedulerInterval
	}
	if cfg.ReconcileInterval <= 0 {
		cfg.ReconcileInterval = defaultReconcileInterval
	}
	if cfg.StatusSyncInterval <= 0 {
		cfg.StatusSyncInterval = defaultStatusSyncInterval
	}
	if cfg.StatusSyncTimeout <= 0 {
		cfg.StatusSyncTimeout = defaultStatusSyncTimeout
	}
	if cfg.StatusSyncBatchSize < 1 {
		cfg.StatusSyncBatchSize = defaultStatusSyncBatchSize
	}
	if cfg.StatusSyncMaxParallel < 1 {
		cfg.StatusSyncMaxParallel = defaultStatusSyncMaxParallel
	}
	if cfg.SandboxOpTimeout <= 0 {
		cfg.SandboxOpTimeout = defaultSandboxOpTimeout
	}
	if cfg.HostPortMin < 1 {
		cfg.HostPortMin = defaultPortMin
	}
	if cfg.HostPortMax < cfg.HostPortMin {
		cfg.HostPortMax = defaultPortMax
	}
	if cfg.CreateRPS <= 0 {
		cfg.CreateRPS = defaultCreateRPS
	}
	if cfg.CreateBurst < 1 {
		cfg.CreateBurst = defaultCreateBurst
	}
	if cfg.HeartbeatInterval <= 0 {
		cfg.HeartbeatInterval = defaultHeartbeatInterval
	}
	if cfg.ProbeTimeout <= 0 {
		cfg.ProbeTimeout = defaultProbeTimeout
	}
	if cfg.ShutdownTimeout <= 0 {
		cfg.ShutdownTimeout = defaultShutdownTimeout
	}
}

type orchFileConfig struct {
	AppEnv                   *string  `json:"app_env,omitempty"`
	HTTPAddr                 *string  `json:"http_addr,omitempty"`
	LogDir                   *string  `json:"log_dir,omitempty"`
	LogFilePrefix            *string  `json:"log_file_prefix,omitempty"`
	SQLitePath               *string  `json:"sqlite_path,omitempty"`
	HeartbeatInterval        *string  `json:"heartbeat_interval,omitempty"`
	ProbeTimeout             *string  `json:"probe_timeout,omitempty"`
	SandboxOpTimeout         *string  `json:"sandbox_op_timeout,omitempty"`
	HeartbeatParallel        *bool    `json:"heartbeat_parallel,omitempty"`
	HeartbeatMaxParallel     *int     `json:"heartbeat_max_parallel,omitempty"`
	ResourceSyncInterval     *string  `json:"resource_sync_interval,omitempty"`
	ResourcePersistMinInt    *string  `json:"resource_persist_min_interval,omitempty"`
	ResourcePersistMaxInt    *string  `json:"resource_persist_max_interval,omitempty"`
	ReadySuccessThreshold    *int     `json:"ready_success_threshold,omitempty"`
	NotReadyFailureThreshold *int     `json:"notready_failure_threshold,omitempty"`
	ShutdownTimeout          *string  `json:"shutdown_timeout,omitempty"`
	SchedulerInterval        *string  `json:"scheduler_interval,omitempty"`
	ReconcileInterval        *string  `json:"reconcile_interval,omitempty"`
	StatusSyncInterval       *string  `json:"status_sync_interval,omitempty"`
	StatusSyncTimeout        *string  `json:"status_sync_timeout,omitempty"`
	StatusSyncBatchSize      *int     `json:"status_sync_batch_size,omitempty"`
	StatusSyncMaxParallel    *int     `json:"status_sync_max_parallel,omitempty"`
	HostPortMin              *int     `json:"host_port_min,omitempty"`
	HostPortMax              *int     `json:"host_port_max,omitempty"`
	CreateRPS                *float64 `json:"create_rps,omitempty"`
	CreateBurst              *int     `json:"create_burst,omitempty"`
}

func (f orchFileConfig) apply(cfg *Config) {
	setString(&cfg.AppEnv, f.AppEnv)
	setString(&cfg.HTTPAddr, f.HTTPAddr)
	setString(&cfg.LogDir, f.LogDir)
	setString(&cfg.LogFilePrefix, f.LogFilePrefix)
	setString(&cfg.SQLitePath, f.SQLitePath)
	setDuration(&cfg.HeartbeatInterval, f.HeartbeatInterval)
	setDuration(&cfg.ProbeTimeout, f.ProbeTimeout)
	setDuration(&cfg.SandboxOpTimeout, f.SandboxOpTimeout)
	if f.HeartbeatParallel != nil {
		cfg.HeartbeatParallel = *f.HeartbeatParallel
	}
	setInt(&cfg.HeartbeatMaxParallel, f.HeartbeatMaxParallel)
	setDuration(&cfg.ResourceSyncInterval, f.ResourceSyncInterval)
	setDuration(&cfg.ResourcePersistMinInt, f.ResourcePersistMinInt)
	setDuration(&cfg.ResourcePersistMaxInt, f.ResourcePersistMaxInt)
	setInt(&cfg.ReadySuccessThreshold, f.ReadySuccessThreshold)
	setInt(&cfg.NotReadyFailureThreshold, f.NotReadyFailureThreshold)
	setDuration(&cfg.ShutdownTimeout, f.ShutdownTimeout)
	setDuration(&cfg.SchedulerInterval, f.SchedulerInterval)
	setDuration(&cfg.ReconcileInterval, f.ReconcileInterval)
	setDuration(&cfg.StatusSyncInterval, f.StatusSyncInterval)
	setDuration(&cfg.StatusSyncTimeout, f.StatusSyncTimeout)
	setInt(&cfg.StatusSyncBatchSize, f.StatusSyncBatchSize)
	setInt(&cfg.StatusSyncMaxParallel, f.StatusSyncMaxParallel)
	setInt(&cfg.HostPortMin, f.HostPortMin)
	setInt(&cfg.HostPortMax, f.HostPortMax)
	if f.CreateRPS != nil {
		cfg.CreateRPS = *f.CreateRPS
	}
	setInt(&cfg.CreateBurst, f.CreateBurst)
}

func setString(dst *string, src *string) {
	if src != nil {
		*dst = *src
	}
}

func setInt(dst *int, src *int) {
	if src != nil {
		*dst = *src
	}
}

func setDuration(dst *time.Duration, src *string) {
	if src == nil {
		return
	}
	if v, err := time.ParseDuration(*src); err == nil {
		*dst = v
	}
}
