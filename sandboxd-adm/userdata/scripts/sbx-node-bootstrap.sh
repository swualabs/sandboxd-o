#!/usr/bin/env bash
set -Eeuo pipefail
IFS=$'\n\t'

DEFAULT_ARTICLES_ZIP_URL="https://github.com/swualabs/sandboxd-o/releases/download/v0.5.0/articles.zip"

ARTICLES_ZIP_URL="${ARTICLES_ZIP_URL:-$DEFAULT_ARTICLES_ZIP_URL}"
APP_ROOT="${APP_ROOT:-/opt/sandboxd-o}"
APP_DIR="${APP_ROOT}/articles"
SYSTEMD_DIR="/etc/systemd/system"

COMPONENT="${1:-${SANDBOXD_COMPONENT:-}}"
START_SERVICE="${START_SERVICE:-true}"

TMP_DIR=""

log() {
    printf '[sbx-node-bootstrap] %s\n' "$*" >&2
}

die() {
    printf '[sbx-node-bootstrap] ERROR: %s\n' "$*" >&2
    exit 1
}

cleanup() {
    if [[ -n "${TMP_DIR:-}" && -d "$TMP_DIR" ]]; then
        rm -rf "$TMP_DIR"
    fi
}

on_error() {
    local exit_code=$?
    local line_no="${1:-unknown}"

    printf '[sbx-node-bootstrap] ERROR: failed at line %s, exit code %s\n' "$line_no" "$exit_code" >&2

    if [[ -n "${COMPONENT:-}" ]] && command -v systemctl >/dev/null 2>&1; then
        if systemctl list-unit-files "${COMPONENT}.service" >/dev/null 2>&1; then
            printf '\n[sbx-node-bootstrap] Recent journal logs:\n' >&2
            journalctl -u "${COMPONENT}.service" -n 120 --no-pager >&2 || true
        fi
    fi

    exit "$exit_code"
}

trap 'on_error $LINENO' ERR
trap cleanup EXIT

usage() {
    cat >&2 <<'EOF'
Usage:
  sudo bash sbx-node-bootstrap.sh sbxlet
  sudo bash sbx-node-bootstrap.sh sbxorch

Environment variables:
  ARTICLES_ZIP_URL    Release zip URL.
  APP_ROOT            Install root directory. Default: /opt/sandboxd-o
  START_SERVICE       Start service immediately after install. Default: true

Examples:
  sudo bash sbx-node-bootstrap.sh sbxlet
  sudo bash sbx-node-bootstrap.sh sbxorch
  sudo START_SERVICE=false bash sbx-node-bootstrap.sh sbxlet
  sudo ARTICLES_ZIP_URL=https://github.com/swualabs/sandboxd-o/releases/download/v0.5.0/articles.zip bash sbx-node-bootstrap.sh sbxlet
EOF
}

require_root() {
    if [[ "${EUID}" -ne 0 ]]; then
        die "this script must be run as root. Use sudo."
    fi
}

normalize_component() {
    case "$COMPONENT" in
        sbxlet | sbxorch)
            ;;
        "" | -h | --help | help)
            usage
            exit 0
            ;;
        *)
            usage
            die "invalid component: ${COMPONENT}. Expected 'sbxlet' or 'sbxorch'."
            ;;
    esac
}

require_ubuntu() {
    if [[ ! -r /etc/os-release ]]; then
        die "/etc/os-release not found. This script expects Ubuntu."
    fi

    # shellcheck disable=SC1091
    . /etc/os-release

    if [[ "${ID:-}" != "ubuntu" ]]; then
        die "unsupported OS: ${ID:-unknown}. This script expects Ubuntu."
    fi
}

require_systemd() {
    if ! command -v systemctl >/dev/null 2>&1; then
        die "systemctl not found. This script requires systemd."
    fi

    if [[ ! -d /run/systemd/system ]]; then
        die "systemd does not appear to be running."
    fi
}

install_packages() {
    export DEBIAN_FRONTEND=noninteractive

    log "Installing required packages"

    apt-get update
    apt-get install -y \
        ca-certificates \
        curl \
        unzip \
        jq \
        tar
}

download_and_extract() {
    local tmp_dir="$1"
    local zip_path="${tmp_dir}/articles.zip"
    local extract_dir="${tmp_dir}/extract"

    mkdir -p "$extract_dir"

    log "Downloading articles.zip from ${ARTICLES_ZIP_URL}"

    curl \
        --fail \
        --location \
        --show-error \
        --silent \
        --retry 5 \
        --retry-delay 2 \
        --connect-timeout 10 \
        --max-time 300 \
        "$ARTICLES_ZIP_URL" \
        -o "$zip_path"

    if [[ ! -s "$zip_path" ]]; then
        die "downloaded zip file is empty: ${zip_path}"
    fi

    log "Validating zip archive"
    unzip -tq "$zip_path" >/dev/null

    log "Extracting zip archive"
    unzip -q "$zip_path" -d "$extract_dir"

    printf '%s\n' "$extract_dir"
}

find_articles_dir() {
    local extract_dir="$1"
    local component_path=""

    if [[ ! -d "$extract_dir" ]]; then
        die "extract directory does not exist: ${extract_dir}"
    fi

    component_path="$(find "$extract_dir" -type f -name "$COMPONENT" | head -n 1 || true)"

    if [[ -z "$component_path" ]]; then
        log "Extracted files:"
        find "$extract_dir" -maxdepth 5 -type f -printf '  %p\n' >&2 || true
        die "component binary '${COMPONENT}' not found in articles.zip"
    fi

    dirname "$component_path"
}

validate_extracted_files() {
    local src_dir="$1"
    local configs_dir="$2"
    local scripts_dir="$3"

    [[ -d "$src_dir" ]] || die "source directory does not exist: ${src_dir}"
    [[ -f "${src_dir}/${COMPONENT}" ]] || die "${COMPONENT} not found in ${src_dir}"

    if [[ "$COMPONENT" == "sbxlet" ]]; then
        [[ -f "${scripts_dir}/install.sh" ]] || die "install.sh is required for sbxlet but was not found in ${scripts_dir}"
    fi

    [[ -f "${configs_dir}/sbxlet_config.json" ]] || die "configs/sbxlet_config.json not found in ${configs_dir}"
    [[ -f "${configs_dir}/sbxorch_config.json" ]] || die "configs/sbxorch_config.json not found in ${configs_dir}"
    [[ -f "${configs_dir}/sbxctl_config.json" ]] || die "configs/sbxctl_config.json not found in ${configs_dir}"
}

stop_existing_services() {
    log "Stopping old sandboxd-o services if present"

    local services=(
        sbxlet.service
        sbxorch.service
        sandboxd-o.service
        sandboxd-o-sbxlet.service
        sandboxd-o-sbxorch.service
    )

    for svc in "${services[@]}"; do
        systemctl stop "$svc" >/dev/null 2>&1 || true
        systemctl disable "$svc" >/dev/null 2>&1 || true

        if [[ -f "${SYSTEMD_DIR}/${svc}" ]]; then
            rm -f "${SYSTEMD_DIR}/${svc}"
        fi
    done

    systemctl daemon-reload
}

install_articles() {
    local src_dir="$1"
    local scripts_dir="$2"

    log "Installing articles into ${APP_DIR}"

    mkdir -p "$APP_ROOT"
    rm -rf "$APP_DIR"
    mkdir -p "$APP_DIR"

    cp -a "${src_dir}/." "$APP_DIR/"

    if [[ "$COMPONENT" == "sbxlet" && -f "${scripts_dir}/install.sh" ]]; then
        cp -a "${scripts_dir}/install.sh" "${APP_DIR}/install.sh"
    fi

    chown -R root:root "$APP_DIR"

    chmod 0755 "$APP_ROOT"
    chmod 0755 "$APP_DIR"
    chmod 0755 "${APP_DIR}/${COMPONENT}"

    if [[ -f "${APP_DIR}/install.sh" ]]; then
        chmod 0755 "${APP_DIR}/install.sh"
    fi
}

install_default_configs() {
    local configs_dir="$1"
    local config_root="/var/lib/sandboxd"

    log "Installing default JSON configs into ${config_root}"

    mkdir -p "${config_root}"
    install -m 0644 "${configs_dir}/sbxlet_config.json" "${config_root}/sbxlet_config.json"
    install -m 0644 "${configs_dir}/sbxorch_config.json" "${config_root}/sbxorch_config.json"
    install -m 0644 "${configs_dir}/sbxctl_config.json" "${config_root}/sbxctl_config.json"
}

run_install_sh_if_needed() {
    if [[ "$COMPONENT" != "sbxlet" ]]; then
        log "Skipping install.sh because selected component is ${COMPONENT}"
        return
    fi

    log "Running install.sh for sbxlet"

    cd "$APP_DIR"
    # install.sh's own last line ("runsc --version | head -n 1") can exit
    # 141 (SIGPIPE) under `set -o pipefail` purely because head closes the
    # pipe early -- the install itself has already fully succeeded by then.
    # Treat that one exit code as success; anything else is a real failure.
    local rc=0
    set +e
    ./install.sh
    rc=$?
    set -e
    if [[ $rc -ne 0 && $rc -ne 141 ]]; then
        die "install.sh failed with exit code ${rc}"
    fi
}

setup_ecr_auto_login() {
    if [[ "$COMPONENT" != "sbxlet" ]]; then
        return
    fi

    log "Configuring automatic ECR registry auth for containerd"

    if ! command -v aws >/dev/null 2>&1; then
        DEBIAN_FRONTEND=noninteractive apt-get install -y awscli >/dev/null
    fi

    # Point containerd's CRI registry resolver at a config_path directory.
    # Host configs under it (certs.d/<host>/hosts.toml) are read per pull,
    # so the periodic token refresh below only rewrites a hosts.toml file
    # and never has to restart containerd. config_path itself is a config
    # change, so it's applied via a conf.d drop-in with a single restart
    # here at provisioning time (before any sandbox is running).
    if ! grep -q '^imports = \["/etc/containerd/conf.d/\*\.toml"\]' /etc/containerd/config.toml 2>/dev/null; then
        sed -i '1i imports = ["/etc/containerd/conf.d/*.toml"]' /etc/containerd/config.toml
    fi
    mkdir -p /etc/containerd/conf.d /etc/containerd/certs.d

    cat >/etc/containerd/conf.d/registry.toml <<'EOF'
[plugins."io.containerd.grpc.v1.cri".registry]
  config_path = "/etc/containerd/certs.d"
EOF

    local region
    region="$(curl -fsS -m 3 -H "X-aws-ec2-metadata-token: $(curl -fsS -m 3 -X PUT -H 'X-aws-ec2-metadata-token-ttl-seconds: 60' http://169.254.169.254/latest/api/token)" http://169.254.169.254/latest/meta-data/placement/region)"
    [[ -n "$region" ]] || die "could not resolve region from instance metadata for ECR auto-login"

    # The refresh script writes the per-host hosts.toml with a Basic auth
    # header (AWS:<ecr-token>). hosts.toml is consulted per pull, so a
    # refreshed token takes effect without restarting containerd.
    cat >/usr/local/bin/sbxadm-ecr-refresh.sh <<SCRIPT
#!/bin/bash
set -euo pipefail
umask 077
REGION="${region}"
ACCOUNT_ID="\$(aws sts get-caller-identity --region "\${REGION}" --query Account --output text)"
TOKEN="\$(aws ecr get-login-password --region "\${REGION}")"
HOST="\${ACCOUNT_ID}.dkr.ecr.\${REGION}.amazonaws.com"
BASIC="\$(printf 'AWS:%s' "\${TOKEN}" | base64 -w0)"

mkdir -p "/etc/containerd/certs.d/\${HOST}"
cat >"/etc/containerd/certs.d/\${HOST}/hosts.toml" <<EOF
server = "https://\${HOST}"

[host."https://\${HOST}"]
  capabilities = ["pull", "resolve"]
  [host."https://\${HOST}".header]
    Authorization = ["Basic \${BASIC}"]
EOF
SCRIPT
    chmod 0700 /usr/local/bin/sbxadm-ecr-refresh.sh

    cat >/etc/systemd/system/sbxadm-ecr-refresh.service <<'EOF'
[Unit]
Description=Refresh ECR registry auth for containerd

[Service]
Type=oneshot
ExecStart=/usr/local/bin/sbxadm-ecr-refresh.sh
EOF

    # ECR tokens last ~12h; refresh every 6h. No containerd restart needed.
    cat >/etc/systemd/system/sbxadm-ecr-refresh.timer <<'EOF'
[Unit]
Description=Periodic ECR registry auth refresh for containerd

[Timer]
OnBootSec=30s
OnUnitActiveSec=6h

[Install]
WantedBy=timers.target
EOF

    # One-time restart so containerd picks up config_path; subsequent token
    # refreshes only rewrite hosts.toml and need no restart.
    systemctl restart containerd
    systemctl daemon-reload
    systemctl enable --now sbxadm-ecr-refresh.timer
    /usr/local/bin/sbxadm-ecr-refresh.sh
}

write_systemd_service() {
    local service_path="${SYSTEMD_DIR}/${COMPONENT}.service"
    local binary_path="${APP_DIR}/${COMPONENT}"

    log "Writing systemd service: ${service_path}"

    cat >"$service_path" <<EOF
[Unit]
Description=Sandboxd-O ${COMPONENT}
Documentation=https://github.com/swualabs/sandboxd-o
Wants=network-online.target
After=network-online.target

[Service]
Type=simple
User=root
Group=root
WorkingDirectory=${APP_DIR}
ExecStart=${binary_path}
Restart=always
RestartSec=3
KillSignal=SIGTERM
TimeoutStopSec=30
LimitNOFILE=1048576
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
EOF

    chmod 0644 "$service_path"

    systemctl daemon-reload
    systemctl enable "${COMPONENT}.service"
}

verify_binary() {
    local binary_path="${APP_DIR}/${COMPONENT}"

    [[ -f "$binary_path" ]] || die "binary not found after install: ${binary_path}"
    [[ -x "$binary_path" ]] || die "binary is not executable: ${binary_path}"

    log "Binary installed: ${binary_path}"
}

start_and_verify_service() {
    if [[ "$START_SERVICE" != "true" ]]; then
        log "Skipping immediate service start because START_SERVICE=${START_SERVICE}"
        return
    fi

    log "Starting ${COMPONENT}.service"

    systemctl restart "${COMPONENT}.service"

    sleep 2

    if ! systemctl is-active --quiet "${COMPONENT}.service"; then
        journalctl -u "${COMPONENT}.service" -n 120 --no-pager >&2 || true
        die "${COMPONENT}.service failed to start"
    fi

    log "${COMPONENT}.service is active"
}

print_summary() {
    cat <<EOF

[sbx-node-bootstrap] Done.

Component:
  ${COMPONENT}

Installed directory:
  ${APP_DIR}

Systemd service:
  ${COMPONENT}.service

Useful commands:
  systemctl status ${COMPONENT}.service --no-pager
  journalctl -u ${COMPONENT}.service -f
  systemctl restart ${COMPONENT}.service

AMI note:
  This service is enabled and will start automatically on boot.
EOF
}

main() {
    require_root
    normalize_component
    require_ubuntu
    require_systemd
    install_packages

    TMP_DIR="$(mktemp -d)"

    local extract_dir
    extract_dir="$(download_and_extract "$TMP_DIR")"

    local src_dir
    src_dir="$(find_articles_dir "$extract_dir")"

    local configs_dir="${extract_dir}/configs"
    local scripts_dir="${extract_dir}/scripts"

    log "Detected articles directory: ${src_dir}"

    validate_extracted_files "$src_dir" "$configs_dir" "$scripts_dir"
    stop_existing_services
    install_articles "$src_dir" "$scripts_dir"
    install_default_configs "$configs_dir"
    run_install_sh_if_needed
    setup_ecr_auto_login
    verify_binary
    write_systemd_service
    start_and_verify_service
    print_summary
}

main "$@"
