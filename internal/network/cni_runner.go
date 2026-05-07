package network

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"

	"github.com/containernetworking/cni/libcni"
	"github.com/containernetworking/cni/pkg/invoke"
	cnitypes "github.com/containernetworking/cni/pkg/types"
	types100 "github.com/containernetworking/cni/pkg/types/100"
	"github.com/vishvananda/netns"
)

const defaultCNIBinDir = "/opt/cni/bin"

func SandboxNetNSPath(sandboxID string) string {
	return filepath.Join("/var/run/netns", sandboxID)
}

func EnsureSandboxNetNS(sandboxID string) (string, error) {
	if sandboxID == "" {
		return "", fmt.Errorf("sandbox id is required")
	}

	if err := os.MkdirAll("/var/run/netns", 0o755); err != nil {
		return "", err
	}
	_ = netns.DeleteNamed(sandboxID)

	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	origNS, err := netns.Get()
	if err != nil {
		return "", err
	}
	defer origNS.Close()

	n, err := netns.NewNamed(sandboxID)
	if err != nil {
		return "", err
	}

	_ = n.Close()
	if err := netns.Set(origNS); err != nil {
		return "", err
	}

	return SandboxNetNSPath(sandboxID), nil
}

func DeleteSandboxNetNS(sandboxID string) {
	if sandboxID == "" {
		return
	}

	_ = netns.DeleteNamed(sandboxID)
}

func CNIAdd(ctx context.Context, sandboxID, netnsPath, confPath string) (string, error) {
	if sandboxID == "" || netnsPath == "" || confPath == "" {
		return "", fmt.Errorf("invalid cni add input")
	}

	cfg, err := libcni.ConfListFromFile(confPath)
	if err != nil {
		return "", fmt.Errorf("load cni conf %s: %w", confPath, err)
	}

	cni := libcni.NewCNIConfigWithCacheDir([]string{defaultCNIBinDir}, "/var/lib/cni", &invoke.DefaultExec{RawExec: &invoke.RawExec{}})
	rt := &libcni.RuntimeConf{
		ContainerID: sandboxID,
		NetNS:       netnsPath,
		IfName:      "eth0",
	}

	res, err := cni.AddNetworkList(ctx, cfg, rt)
	if err != nil {
		return "", fmt.Errorf("cni add (%s): %w", cfg.Name, err)
	}

	ip, err := firstIPv4FromResult(res)
	if err != nil {
		return "", err
	}

	return ip, nil
}

func CNIDel(ctx context.Context, sandboxID, netnsPath, confPath string) {
	if sandboxID == "" || netnsPath == "" || confPath == "" {
		return
	}

	cfg, err := libcni.ConfListFromFile(confPath)
	if err != nil {
		return
	}

	cni := libcni.NewCNIConfigWithCacheDir([]string{defaultCNIBinDir}, "/var/lib/cni", &invoke.DefaultExec{RawExec: &invoke.RawExec{}})
	rt := &libcni.RuntimeConf{ContainerID: sandboxID, NetNS: netnsPath, IfName: "eth0"}
	_ = cni.DelNetworkList(ctx, cfg, rt)
}

func firstIPv4FromResult(res cnitypes.Result) (string, error) {
	r, err := types100.NewResultFromResult(res)
	if err == nil {
		for _, ipConf := range r.IPs {
			if ipConf.Address.IP.To4() != nil {
				return ipConf.Address.IP.String(), nil
			}
		}
	}

	// Fallback parser for plugin outputs that don't map cleanly to types/100.
	raw, err := json.Marshal(res)
	if err != nil {
		return "", fmt.Errorf("marshal cni result: %w", err)
	}

	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return "", fmt.Errorf("unmarshal cni result: %w", err)
	}

	// CNI 0.3-style: {"ip4":{"ip":"10.89.0.2/16"}}
	if ip4, ok := m["ip4"].(map[string]any); ok {
		if ipCIDR, ok := ip4["ip"].(string); ok {
			if ip := parseIPv4CIDR(ipCIDR); ip != "" {
				return ip, nil
			}
		}
	}

	// CNI 1.0-style: {"ips":[{"address":"10.89.0.2/16"}]}
	if ips, ok := m["ips"].([]any); ok {
		for _, item := range ips {
			entry, ok := item.(map[string]any)
			if !ok {
				continue
			}

			if addr, ok := entry["address"].(string); ok {
				if ip := parseIPv4CIDR(addr); ip != "" {
					return ip, nil
				}
			}
		}
	}

	return "", fmt.Errorf("no ipv4 in cni add result")
}

func parseIPv4CIDR(v string) string {
	ip, _, err := net.ParseCIDR(v)
	if err != nil || ip == nil {
		return ""
	}

	ip4 := ip.To4()
	if ip4 == nil {
		return ""
	}

	return ip4.String()
}
