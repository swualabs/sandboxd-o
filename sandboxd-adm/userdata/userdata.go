// Package userdata builds the EC2 UserData (cloud-init) bootstrap script
// that turns a bare Ubuntu instance into an sbxorch control plane or sbxlet
// worker node, by running the embedded scripts/sbx-node-bootstrap.sh
// directly on the live instance at boot time (no AMI-baking step).
package userdata

import (
	"bytes"
	"compress/gzip"
	_ "embed"
	"encoding/base64"
	"fmt"
	"strings"
)

//go:embed scripts/sbx-node-bootstrap.sh
var bootstrapScript []byte

//go:embed defaultconfigs/sbxorch_config.json
var defaultOrchConfig []byte

//go:embed defaultconfigs/sbxlet_config.json
var defaultLetConfig []byte

//go:embed defaultconfigs/sbxctl_config.json
var defaultCtlConfig []byte

var defaultConfigs = map[string][]byte{
	"sbxorch": defaultOrchConfig,
	"sbxlet":  defaultLetConfig,
	"sbxctl":  defaultCtlConfig,
}

// The release asset filename has changed across sandboxd-o versions
// (articles.zip, sbx-build-artifacts-x86_64.zip, ...), so instead of
// hardcoding a download URL we resolve the actual *.zip asset for the
// requested tag via the GitHub releases API at boot time.
const releaseAPITemplate = "https://api.github.com/repos/swualabs/sandboxd-o/releases/tags/v%s"

type Params struct {
	Component  string // "sbxorch" or "sbxlet"
	Version    string // e.g. "0.3.0"
	ConfigJSON string // final merged JSON config to install at /var/lib/sandboxd/<component>_config.json
}

func Render(p Params) (string, error) {
	if p.Component != "sbxorch" && p.Component != "sbxlet" {
		return "", fmt.Errorf("unsupported component %q", p.Component)
	}

	if strings.TrimSpace(p.Version) == "" {
		return "", fmt.Errorf("version is required")
	}

	if strings.TrimSpace(p.ConfigJSON) == "" {
		return "", fmt.Errorf("config json is required")
	}

	releaseAPI := fmt.Sprintf(releaseAPITemplate, strings.TrimPrefix(p.Version, "v"))
	gzScript, err := gzipBytes(bootstrapScript)
	if err != nil {
		return "", fmt.Errorf("gzip bootstrap script: %w", err)
	}
	scriptB64 := base64.StdEncoding.EncodeToString(gzScript)
	configB64 := base64.StdEncoding.EncodeToString([]byte(p.ConfigJSON))

	var b strings.Builder
	fmt.Fprintf(&b, "#!/bin/bash\n")
	fmt.Fprintf(&b, "set -euo pipefail\n")
	fmt.Fprintf(&b, "exec > >(tee -a /var/log/sbxadm-userdata.log) 2>&1\n")
	fmt.Fprintf(&b, "echo \"[sbxadm] bootstrap start component=%s version=%s\"\n\n", p.Component, p.Version)

	fmt.Fprintf(&b, "MARKER=/var/lib/sandboxd/.sbxadm-bootstrap-%s\n", p.Component)
	fmt.Fprintf(&b, "TARGET_VERSION=%q\n", p.Version)
	fmt.Fprintf(&b, "if [ -f \"$MARKER\" ] && [ \"$(cat \"$MARKER\")\" = \"$TARGET_VERSION\" ] && systemctl is-active --quiet %s.service; then\n", p.Component)
	fmt.Fprintf(&b, "  echo \"[sbxadm] %s already bootstrapped at version $TARGET_VERSION, skipping install\"\n", p.Component)
	fmt.Fprintf(&b, "else\n")
	fmt.Fprintf(&b, "  mkdir -p /opt/sbxadm-bootstrap\n")
	fmt.Fprintf(&b, "  echo %s | base64 -d | gunzip > /opt/sbxadm-bootstrap/sbx-node-bootstrap.sh\n", scriptB64)
	fmt.Fprintf(&b, "  chmod 0755 /opt/sbxadm-bootstrap/sbx-node-bootstrap.sh\n")
	fmt.Fprintf(&b, "  echo \"[sbxadm] resolving release asset for v%s\"\n", p.Version)
	fmt.Fprintf(&b, "  RELEASE_JSON=$(curl -fsSL --retry 5 --retry-delay 2 %q)\n", releaseAPI)
	fmt.Fprintf(&b, "  ARTICLES_ZIP_URL=$(printf '%%s' \"$RELEASE_JSON\" | grep -o '\"browser_download_url\": *\"[^\"]*\\.zip\"' | head -n1 | sed -E 's/.*\"(https[^\"]+)\"/\\1/')\n")
	fmt.Fprintf(&b, "  if [ -z \"$ARTICLES_ZIP_URL\" ]; then\n")
	fmt.Fprintf(&b, "    echo \"[sbxadm] ERROR: no .zip release asset found for v%s\" >&2\n", p.Version)
	fmt.Fprintf(&b, "    exit 1\n")
	fmt.Fprintf(&b, "  fi\n")
	fmt.Fprintf(&b, "  echo \"[sbxadm] using release asset: $ARTICLES_ZIP_URL\"\n")
	fmt.Fprintf(&b, "  ARTICLES_ZIP_URL=\"$ARTICLES_ZIP_URL\" SANDBOXD_COMPONENT=%s bash /opt/sbxadm-bootstrap/sbx-node-bootstrap.sh %s\n", p.Component, p.Component)
	fmt.Fprintf(&b, "  echo \"$TARGET_VERSION\" > \"$MARKER\"\n")
	fmt.Fprintf(&b, "fi\n\n")

	fmt.Fprintf(&b, "echo \"[sbxadm] installing %s config\"\n", p.Component)
	fmt.Fprintf(&b, "mkdir -p /var/lib/sandboxd\n")
	fmt.Fprintf(&b, "echo %s | base64 -d > /var/lib/sandboxd/%s_config.json\n", configB64, p.Component)
	fmt.Fprintf(&b, "chmod 0644 /var/lib/sandboxd/%s_config.json\n\n", p.Component)

	fmt.Fprintf(&b, "echo \"[sbxadm] restarting %s.service with final config\"\n", p.Component)
	fmt.Fprintf(&b, "systemctl restart %s.service\n", p.Component)
	fmt.Fprintf(&b, "sleep 2\n")
	fmt.Fprintf(&b, "if ! systemctl is-active --quiet %s.service; then\n", p.Component)
	fmt.Fprintf(&b, "  journalctl -u %s.service -n 200 --no-pager || true\n", p.Component)
	fmt.Fprintf(&b, "  echo \"[sbxadm] ERROR: %s.service failed to become active after config install\"\n", p.Component)
	fmt.Fprintf(&b, "  exit 1\n")
	fmt.Fprintf(&b, "fi\n")
	fmt.Fprintf(&b, "echo \"[sbxadm] bootstrap complete component=%s version=%s\"\n", p.Component, p.Version)

	return b.String(), nil
}

// gzipBytes keeps the embedded script + rendered wrapper + config under
// EC2's 16KB UserData limit.
func gzipBytes(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	w, err := gzip.NewWriterLevel(&buf, gzip.BestCompression)
	if err != nil {
		return nil, err
	}

	if _, err := w.Write(data); err != nil {
		return nil, err
	}

	if err := w.Close(); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}
