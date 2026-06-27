package awsx

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	ssmtypes "github.com/aws/aws-sdk-go-v2/service/ssm/types"
)

func WaitForSSMOnline(ctx context.Context, c *ssm.Client, instanceID string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		out, err := c.DescribeInstanceInformation(ctx, &ssm.DescribeInstanceInformationInput{
			Filters: []ssmtypes.InstanceInformationStringFilter{
				{Key: aws.String("InstanceIds"), Values: []string{instanceID}},
			},
		})
		if err == nil && len(out.InstanceInformationList) > 0 && out.InstanceInformationList[0].PingStatus == ssmtypes.PingStatusOnline {
			return nil
		}

		if time.Now().After(deadline) {
			return fmt.Errorf("timed out waiting for SSM agent on %s to come online", instanceID)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(5 * time.Second):
		}
	}
}

// RunShellCommand does not fail on a non-zero remote exit code; check the
// returned status.
func RunShellCommand(ctx context.Context, c *ssm.Client, instanceID string, commands []string, timeout time.Duration) (stdout string, status string, err error) {
	send, err := c.SendCommand(ctx, &ssm.SendCommandInput{
		InstanceIds:  []string{instanceID},
		DocumentName: aws.String("AWS-RunShellScript"),
		Parameters:   map[string][]string{"commands": commands},
	})
	if err != nil {
		return "", "", fmt.Errorf("send ssm command to %s: %w", instanceID, err)
	}
	commandID := aws.ToString(send.Command.CommandId)

	deadline := time.Now().Add(timeout)
	for {
		inv, err := c.GetCommandInvocation(ctx, &ssm.GetCommandInvocationInput{
			CommandId:  aws.String(commandID),
			InstanceId: aws.String(instanceID),
		})
		if err == nil {
			switch inv.Status {
			case ssmtypes.CommandInvocationStatusSuccess, ssmtypes.CommandInvocationStatusFailed,
				ssmtypes.CommandInvocationStatusCancelled, ssmtypes.CommandInvocationStatusTimedOut:
				return aws.ToString(inv.StandardOutputContent), string(inv.Status), nil
			}
		}

		if time.Now().After(deadline) {
			return "", "", fmt.Errorf("timed out waiting for ssm command %s on %s", commandID, instanceID)
		}

		select {
		case <-ctx.Done():
			return "", "", ctx.Err()
		case <-time.After(4 * time.Second):
		}
	}
}

// WaitForLocalHealthz polls 127.0.0.1:<port>/healthz from inside the
// instance via SSM, so it works regardless of whether the node has a
// public IP.
func WaitForLocalHealthz(ctx context.Context, c *ssm.Client, instanceID string, port int, timeout time.Duration) error {
	deadlineSecs := int(timeout.Seconds())
	if deadlineSecs < 30 {
		deadlineSecs = 30
	}

	// POSIX sh (not bash) is what AWS-RunShellScript actually executes on
	// Ubuntu (/bin/sh -> dash), so this must not rely on bash-only
	// features like the magic $SECONDS variable -- use `date +%s` instead.
	script := fmt.Sprintf(`
end=$(( $(date +%%s) + %d ))
while [ "$(date +%%s)" -lt "$end" ]; do
  if curl -fsS -m 3 http://127.0.0.1:%d/healthz >/dev/null 2>&1; then
    echo HEALTHY
    exit 0
  fi
  sleep 5
done
echo UNHEALTHY
exit 1
`, deadlineSecs, port)

	stdout, status, err := RunShellCommand(ctx, c, instanceID, []string{script}, timeout+30*time.Second)
	if err != nil {
		return err
	}

	if status != "Success" || !strings.Contains(stdout, "HEALTHY") {
		return fmt.Errorf("service on %s:%d did not become healthy within %s (ssm status=%s)", instanceID, port, timeout, status)
	}

	return nil
}
