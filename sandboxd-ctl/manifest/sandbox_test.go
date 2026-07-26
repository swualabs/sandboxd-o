package manifest

import "testing"

func TestParseManifest_SandboxEphemeralStorage(t *testing.T) {
	raw := []byte(`
apiVersion: sandboxd.o/v1
kind: Sandbox
id: sbx-a
spec:
  egress: true
  containers:
    - name: app
      image: alpine:3.20
      resource:
        cpu: 100m
        memory: 64Mi
        ephemeral_storage: 96Mi
`)
	out, err := ParseManifest(raw)
	if err != nil {
		t.Fatalf("ParseManifest err=%v", err)
	}
	spec, _ := out["spec"].(map[string]any)
	containers, _ := spec["containers"].([]any)
	if len(containers) != 1 {
		t.Fatalf("containers=%d", len(containers))
	}
	c0, _ := containers[0].(map[string]any)
	res, _ := c0["resource"].(map[string]any)
	if got, _ := res["ephemeral_storage"].(string); got != "96Mi" {
		t.Fatalf("ephemeral_storage=%q", got)
	}
}

func TestParseManifest_NodeMetadataLabels(t *testing.T) {
	raw := []byte(`
apiVersion: sandboxd.o/v1
kind: Node
id: n1
metadata:
  labels:
    node-type: example
    region: ap-northeast-2
spec:
  ip: 127.0.0.1
  port: 8081
  unschedulable: false
`)
	out, err := ParseManifest(raw)
	if err != nil {
		t.Fatalf("ParseManifest err=%v", err)
	}

	metadata, _ := out["metadata"].(map[string]any)
	labels, _ := metadata["labels"].(map[string]any)
	if labels["node-type"] != "example" || labels["region"] != "ap-northeast-2" {
		t.Fatalf("labels=%v", labels)
	}
}
