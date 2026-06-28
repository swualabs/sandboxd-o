package orchestrate

import (
	"context"
	"encoding/base64"
	"fmt"
	"time"

	"sandboxd-o/sandboxd-adm/awsx"
	"sandboxd-o/sandboxd-adm/stepper"
	"sandboxd-o/sandboxd-adm/store"
	"sandboxd-o/sandboxd-adm/userdata"

	"github.com/aws/aws-sdk-go-v2/service/ssm"
)

// UpdateSbxctlConfig pushes a freshly merged sbxctl_config.json to a
// cluster's control plane node over SSM. Only allowed when the control
// plane has a public endpoint, since /var/lib/sandboxd/sbxctl_config.json
// on the orch node is meant for operators reaching it directly; a
// VPC-internal-only control plane has no business exposing that file this
// way (use the cluster's stored shared secret + control plane IP from
// `sbxadm info cluster` instead).
func UpdateSbxctlConfig(ctx context.Context, ssmc *ssm.Client, st *store.Store, clusterName string, s *stepper.Stepper) error {
	s.Step("loading cluster %q", clusterName)
	cluster, err := st.GetCluster(ctx, clusterName)
	if err != nil {
		return err
	}

	if !cluster.ControlPlane.PublicEndpoint {
		return fmt.Errorf("cluster %q control plane is not publicly accessible; update-sbxctl-config requires --orch-public-endpoint at cluster creation time", clusterName)
	}
	s.Done("control plane %s is publicly accessible", cluster.ControlPlane.InstanceID)

	server := fmt.Sprintf("http://%s:%d", cluster.ControlPlane.PublicIP, orchAPIPort)

	s.Step("merging sbxctl_config.json (server=%s)", server)
	configJSON, err := userdata.MergeConfig("sbxctl", "", cluster.SharedSecret, map[string]any{
		"server": server,
	})

	if err != nil {
		return fmt.Errorf("build sbxctl config: %w", err)
	}
	s.Done("config merged")

	s.Step("pushing sbxctl_config.json to control plane %s via SSM", cluster.ControlPlane.InstanceID)
	b64 := base64.StdEncoding.EncodeToString([]byte(configJSON))
	commands := []string{
		"mkdir -p /var/lib/sandboxd",
		fmt.Sprintf("echo %s | base64 -d > /var/lib/sandboxd/sbxctl_config.json", b64),
		"chmod 0644 /var/lib/sandboxd/sbxctl_config.json",
		"echo DONE",
	}

	stdout, status, err := awsx.RunShellCommand(ctx, ssmc, cluster.ControlPlane.InstanceID, commands, 30*time.Second)
	if err != nil {
		return err
	}

	if status != "Success" {
		return fmt.Errorf("pushing sbxctl_config.json failed (ssm status=%s): %s", status, stdout)
	}
	s.Done("sbxctl_config.json updated on %s", cluster.ControlPlane.InstanceID)

	return nil
}
