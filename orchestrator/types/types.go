package types

import "time"

type NodeState string

const (
	NodeStateUnknown  NodeState = "Unknown"
	NodeStateReady    NodeState = "Ready"
	NodeStateNotReady NodeState = "NotReady"
)

type Node struct {
	Name            string     `json:"name"`
	IP              string     `json:"ip"`
	Port            int        `json:"port"`
	State           NodeState  `json:"state"`
	Source          string     `json:"source"`
	LastError       string     `json:"last_error,omitempty"`
	SuccessStreak   int        `json:"success_streak"`
	FailureStreak   int        `json:"failure_streak"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
	LastHeartbeat   *time.Time `json:"last_heartbeat,omitempty"`
	SandboxdBaseURL string     `json:"sandboxd_base_url"`
}

type RegisterNodeRequest struct {
	Name string `json:"name"`
	IP   string `json:"ip"`
	Port int    `json:"port"`
}

type APIServerConfig struct {
	ListenAddress string       `yaml:"listenAddress"`
	Nodes         []StaticNode `yaml:"nodes"`
}

type StaticNode struct {
	Name string `yaml:"name"`
	IP   string `yaml:"ip"`
	Port int    `yaml:"port"`
}
