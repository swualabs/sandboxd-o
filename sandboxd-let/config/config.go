package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"sandboxd-o/pkg/envutil"

	"k8s.io/apimachinery/pkg/api/resource"
)

const (
	DefaultConfigPath             = "/var/lib/sandboxd/sbxlet_config.json"
	DefaultContainerdAddr         = "/run/containerd/containerd.sock"
	DefaultStateBaseDir           = "/var/lib/sandboxd/sandboxes"
	DefaultLockDir                = "/var/lib/sandboxd/locks"
	DefaultBridgeInterface        = "sbx-br0"
	DefaultSubnetCIDR             = "10.89.0.0/16"
	DefaultCNIConfPath            = "/etc/cni/sandboxd.d/20-sbxnet.conflist"
	DefaultRuntimeBinary          = "runsc"
	DefaultReadyTimeout           = 20 * time.Second
	DefaultReconcileEvery         = 15 * time.Second
	DefaultReconcileGrace         = 5 * time.Second
	DefaultReconcileHits          = 2
	DefaultLockWaitTimeout        = 20 * time.Second
	DefaultProvisionTimeout       = 4 * time.Minute
	DefaultContainerCreateTimeout = 2 * time.Minute
	DefaultImagePullTimeout       = 8 * time.Minute
)

type Config struct {
	AppEnv                 string        `json:"app_env,omitempty"`
	HTTPAddr               string        `json:"http_addr,omitempty"`
	LogDir                 string        `json:"log_dir,omitempty"`
	LogFilePrefix          string        `json:"log_file_prefix,omitempty"`
	ContainerdAddress      string        `json:"containerd_address,omitempty"`
	StateBaseDir           string        `json:"state_base_dir,omitempty"`
	LockDir                string        `json:"lock_dir,omitempty"`
	CgroupParent           string        `json:"cgroup_parent,omitempty"`
	BridgeInterface        string        `json:"bridge_interface,omitempty"`
	SubnetCIDR             string        `json:"subnet_cidr,omitempty"`
	CNIConfPath            string        `json:"cni_conf_path,omitempty"`
	ForwardHookChains      []string      `json:"forward_hook_chains,omitempty"`
	ReconcileInterval      time.Duration `json:"reconcile_interval,omitempty"`
	ReconcileGrace         time.Duration `json:"reconcile_grace,omitempty"`
	ReconcileHits          int           `json:"reconcile_hits,omitempty"`
	RuntimeBinary          string        `json:"runtime_binary,omitempty"`
	DefaultEphemeralBytes  int64         `json:"default_ephemeral_bytes,omitempty"`
	RootfsRatioPercent     int           `json:"rootfs_ratio_percent,omitempty"`
	TmpfsRatioPercent      int           `json:"tmpfs_ratio_percent,omitempty"`
	MaxAllocPercent        int           `json:"max_alloc_percent,omitempty"`
	ProvisionTimeout       time.Duration `json:"provision_timeout,omitempty"`
	ContainerCreateTimeout time.Duration `json:"container_create_timeout,omitempty"`
	ImagePullTimeout       time.Duration `json:"image_pull_timeout,omitempty"`
}

func DefaultConfig() Config {
	return Config{
		HTTPAddr:               ":8080",
		LogFilePrefix:          "sandboxd",
		ContainerdAddress:      DefaultContainerdAddr,
		StateBaseDir:           DefaultStateBaseDir,
		LockDir:                DefaultLockDir,
		BridgeInterface:        DefaultBridgeInterface,
		SubnetCIDR:             DefaultSubnetCIDR,
		CNIConfPath:            DefaultCNIConfPath,
		ForwardHookChains:      []string{"FORWARD", "DOCKER-USER"},
		ReconcileInterval:      DefaultReconcileEvery,
		ReconcileGrace:         DefaultReconcileGrace,
		ReconcileHits:          DefaultReconcileHits,
		RuntimeBinary:          DefaultRuntimeBinary,
		DefaultEphemeralBytes:  128 * 1024 * 1024,
		RootfsRatioPercent:     80,
		TmpfsRatioPercent:      20,
		MaxAllocPercent:        90,
		ProvisionTimeout:       DefaultProvisionTimeout,
		ContainerCreateTimeout: DefaultContainerCreateTimeout,
		ImagePullTimeout:       DefaultImagePullTimeout,
	}
}

func Load(path string) (Config, error) {
	cfg := DefaultConfig()
	if err := loadFile(path, &cfg); err != nil {
		return Config{}, err
	}
	applyEnvOverrides(&cfg)
	return WithConfigDefaults(cfg), nil
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

	var fileCfg sbxletFileConfig
	if err := json.Unmarshal(raw, &fileCfg); err != nil {
		return fmt.Errorf("parse config file %q: %w", path, err)
	}
	fileCfg.apply(cfg)

	return nil
}

func applyEnvOverrides(cfg *Config) {
	cfg.AppEnv = envutil.Get("APP_ENV", cfg.AppEnv)
	cfg.HTTPAddr = envutil.Get("HTTP_ADDR", cfg.HTTPAddr)
	cfg.LogDir = envutil.Get("SANDBOX_LOG_DIR", cfg.LogDir)
	cfg.LogFilePrefix = envutil.Get("SANDBOX_LOG_FILE_PREFIX", cfg.LogFilePrefix)
	cfg.ContainerdAddress = envutil.Get("SANDBOX_CONTAINERD_ADDRESS", cfg.ContainerdAddress)
	cfg.StateBaseDir = envutil.Get("SANDBOX_STATE_BASE_DIR", cfg.StateBaseDir)
	cfg.LockDir = envutil.Get("SANDBOX_LOCK_DIR", cfg.LockDir)
	cfg.CgroupParent = envutil.Get("SANDBOX_CGROUP_PARENT", cfg.CgroupParent)
	cfg.BridgeInterface = envutil.Get("SANDBOX_BRIDGE_INTERFACE", cfg.BridgeInterface)
	cfg.SubnetCIDR = envutil.Get("SANDBOX_SUBNET_CIDR", cfg.SubnetCIDR)
	cfg.CNIConfPath = envutil.Get("SANDBOX_CNI_CONF_PATH", cfg.CNIConfPath)
	cfg.ForwardHookChains = csvEnvOrDefault("SANDBOX_FORWARD_HOOK_CHAINS", cfg.ForwardHookChains)
	cfg.RuntimeBinary = envutil.Get("SANDBOX_RUNTIME_BINARY", cfg.RuntimeBinary)
	cfg.DefaultEphemeralBytes = parseSizeBytes(envutil.Get("SANDBOX_DEFAULT_EPHEMERAL_STORAGE", ""), cfg.DefaultEphemeralBytes)
	cfg.RootfsRatioPercent = envutil.GetInt("SANDBOX_EPHEMERAL_ROOTFS_PERCENT", cfg.RootfsRatioPercent)
	cfg.TmpfsRatioPercent = envutil.GetInt("SANDBOX_EPHEMERAL_TMP_PERCENT", cfg.TmpfsRatioPercent)
	cfg.MaxAllocPercent = envutil.GetInt("SANDBOX_MAX_ALLOC_PERCENT", cfg.MaxAllocPercent)
	cfg.ProvisionTimeout = envutil.GetDuration("SANDBOX_PROVISION_TIMEOUT", cfg.ProvisionTimeout)
	cfg.ContainerCreateTimeout = envutil.GetDuration("SANDBOX_CONTAINER_CREATE_TIMEOUT", cfg.ContainerCreateTimeout)
	cfg.ImagePullTimeout = envutil.GetDuration("SANDBOX_IMAGE_PULL_TIMEOUT", cfg.ImagePullTimeout)
}

func parseSizeBytes(raw string, def int64) int64 {
	q, err := resource.ParseQuantity(raw)
	if err != nil {
		return def
	}

	v := q.Value()
	if v <= 0 {
		return def
	}

	return v
}

func WithConfigDefaults(cfg Config) Config {
	if cfg.ContainerdAddress == "" {
		cfg.ContainerdAddress = DefaultContainerdAddr
	}

	if cfg.HTTPAddr == "" {
		cfg.HTTPAddr = ":8080"
	}

	if cfg.LogFilePrefix == "" {
		cfg.LogFilePrefix = "sandboxd"
	}

	if cfg.StateBaseDir == "" {
		cfg.StateBaseDir = DefaultStateBaseDir
	}

	if cfg.LockDir == "" {
		cfg.LockDir = DefaultLockDir
	}

	if cfg.BridgeInterface == "" {
		cfg.BridgeInterface = DefaultBridgeInterface
	}

	if cfg.SubnetCIDR == "" {
		cfg.SubnetCIDR = DefaultSubnetCIDR
	}

	if cfg.CNIConfPath == "" {
		cfg.CNIConfPath = DefaultCNIConfPath
	}

	if len(cfg.ForwardHookChains) == 0 {
		cfg.ForwardHookChains = []string{"FORWARD", "DOCKER-USER"}
	}

	if cfg.ReconcileInterval <= 0 {
		cfg.ReconcileInterval = DefaultReconcileEvery
	}

	if cfg.ReconcileGrace <= 0 {
		cfg.ReconcileGrace = DefaultReconcileGrace
	}

	if cfg.ReconcileHits < 1 {
		cfg.ReconcileHits = DefaultReconcileHits
	}

	if cfg.RuntimeBinary == "" {
		cfg.RuntimeBinary = DefaultRuntimeBinary
	}

	if cfg.DefaultEphemeralBytes <= 0 {
		cfg.DefaultEphemeralBytes = 128 * 1024 * 1024
	}

	if cfg.RootfsRatioPercent <= 0 || cfg.TmpfsRatioPercent < 0 || cfg.RootfsRatioPercent+cfg.TmpfsRatioPercent != 100 {
		cfg.RootfsRatioPercent = 80
		cfg.TmpfsRatioPercent = 20
	}

	if cfg.MaxAllocPercent <= 0 || cfg.MaxAllocPercent > 100 {
		cfg.MaxAllocPercent = 90
	}

	if cfg.ProvisionTimeout <= 0 {
		cfg.ProvisionTimeout = DefaultProvisionTimeout
	}

	if cfg.ContainerCreateTimeout <= 0 {
		cfg.ContainerCreateTimeout = DefaultContainerCreateTimeout
	}

	if cfg.ImagePullTimeout <= 0 {
		cfg.ImagePullTimeout = DefaultImagePullTimeout
	}

	return cfg
}

func csvEnvOrDefault(key string, def []string) []string {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return append([]string(nil), def...)
	}

	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	if len(out) == 0 {
		return append([]string(nil), def...)
	}
	return out
}

type sbxletFileConfig struct {
	AppEnv                  *string  `json:"app_env,omitempty"`
	HTTPAddr                *string  `json:"http_addr,omitempty"`
	LogDir                  *string  `json:"log_dir,omitempty"`
	LogFilePrefix           *string  `json:"log_file_prefix,omitempty"`
	ContainerdAddress       *string  `json:"containerd_address,omitempty"`
	StateBaseDir            *string  `json:"state_base_dir,omitempty"`
	LockDir                 *string  `json:"lock_dir,omitempty"`
	CgroupParent            *string  `json:"cgroup_parent,omitempty"`
	BridgeInterface         *string  `json:"bridge_interface,omitempty"`
	SubnetCIDR              *string  `json:"subnet_cidr,omitempty"`
	CNIConfPath             *string  `json:"cni_conf_path,omitempty"`
	ForwardHookChains       []string `json:"forward_hook_chains,omitempty"`
	ReconcileInterval       *string  `json:"reconcile_interval,omitempty"`
	ReconcileGrace          *string  `json:"reconcile_grace,omitempty"`
	ReconcileHits           *int     `json:"reconcile_hits,omitempty"`
	RuntimeBinary           *string  `json:"runtime_binary,omitempty"`
	DefaultEphemeralStorage *string  `json:"default_ephemeral_storage,omitempty"`
	RootfsRatioPercent      *int     `json:"rootfs_ratio_percent,omitempty"`
	TmpfsRatioPercent       *int     `json:"tmpfs_ratio_percent,omitempty"`
	MaxAllocPercent         *int     `json:"max_alloc_percent,omitempty"`
	ProvisionTimeout        *string  `json:"provision_timeout,omitempty"`
	ContainerCreateTimeout  *string  `json:"container_create_timeout,omitempty"`
	ImagePullTimeout        *string  `json:"image_pull_timeout,omitempty"`
}

func (f sbxletFileConfig) apply(cfg *Config) {
	assignString(&cfg.AppEnv, f.AppEnv)
	assignString(&cfg.HTTPAddr, f.HTTPAddr)
	assignString(&cfg.LogDir, f.LogDir)
	assignString(&cfg.LogFilePrefix, f.LogFilePrefix)
	assignString(&cfg.ContainerdAddress, f.ContainerdAddress)
	assignString(&cfg.StateBaseDir, f.StateBaseDir)
	assignString(&cfg.LockDir, f.LockDir)
	assignString(&cfg.CgroupParent, f.CgroupParent)
	assignString(&cfg.BridgeInterface, f.BridgeInterface)
	assignString(&cfg.SubnetCIDR, f.SubnetCIDR)
	assignString(&cfg.CNIConfPath, f.CNIConfPath)
	if f.ForwardHookChains != nil {
		cfg.ForwardHookChains = append([]string(nil), f.ForwardHookChains...)
	}
	assignDuration(&cfg.ReconcileInterval, f.ReconcileInterval)
	assignDuration(&cfg.ReconcileGrace, f.ReconcileGrace)
	assignInt(&cfg.ReconcileHits, f.ReconcileHits)
	assignString(&cfg.RuntimeBinary, f.RuntimeBinary)
	if f.DefaultEphemeralStorage != nil {
		cfg.DefaultEphemeralBytes = parseSizeBytes(*f.DefaultEphemeralStorage, cfg.DefaultEphemeralBytes)
	}
	assignInt(&cfg.RootfsRatioPercent, f.RootfsRatioPercent)
	assignInt(&cfg.TmpfsRatioPercent, f.TmpfsRatioPercent)
	assignInt(&cfg.MaxAllocPercent, f.MaxAllocPercent)
	assignDuration(&cfg.ProvisionTimeout, f.ProvisionTimeout)
	assignDuration(&cfg.ContainerCreateTimeout, f.ContainerCreateTimeout)
	assignDuration(&cfg.ImagePullTimeout, f.ImagePullTimeout)
}

func assignString(dst *string, src *string) {
	if src != nil {
		*dst = *src
	}
}

func assignInt(dst *int, src *int) {
	if src != nil {
		*dst = *src
	}
}

func assignDuration(dst *time.Duration, src *string) {
	if src == nil {
		return
	}
	if v, err := time.ParseDuration(*src); err == nil {
		*dst = v
	}
}
