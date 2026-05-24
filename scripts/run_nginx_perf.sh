#!/usr/bin/env bash
set -euo pipefail

ORCH_URL=${ORCH_URL:-http://127.0.0.1:8082}
ITER=${ITER:-40}
POLL_MS=${POLL_MS:-100}
PREFIX=${PREFIX:-perf-ng-$(date +%s)}
LOG_FILE=${LOG_FILE:-/home/user/workspace/go-gvisor-containerd-let/.perf/logs/sandboxd-$(date -u +%Y%m%d-%H).log}
OUT_DIR=${OUT_DIR:-/home/user/workspace/go-gvisor-containerd-let/.perf/results/${PREFIX}}
mkdir -p "$OUT_DIR"

CLIENT_CSV="$OUT_DIR/client.csv"
PROV_JSONL="$OUT_DIR/provision.jsonl"
STAGE_JSONL="$OUT_DIR/stage.jsonl"
SUMMARY_MD="$OUT_DIR/summary.md"

printf "iter,id,create_http_ms,ready_ms,phase,polls\n" > "$CLIENT_CSV"

for i in $(seq 1 "$ITER"); do
  id="${PREFIX}-${i}"
  req="$OUT_DIR/$id.json"
  cat > "$req" <<JSON
{"id":"$id","spec":{"egress":false,"ports":[{"container_port":80,"protocol":"tcp"}],"containers":[{"name":"nginx","image":"nginx:latest","resource":{"cpu":"200m","memory":"256Mi","ephemeral_storage":"128Mi"}}]}}
JSON

  t0=$(date +%s%3N)
  curl -sS -X POST "$ORCH_URL/api/v1/sandboxes" -H 'content-type: application/json' --data-binary @"$req" >/dev/null
  t1=$(date +%s%3N)

  phase="Pending"
  polls=0
  while true; do
    polls=$((polls+1))
    phase=$(curl -sS "$ORCH_URL/api/v1/sandboxes/$id" | jq -r '.sandbox.status.phase')
    if [[ "$phase" == "Running" || "$phase" == "Failed" || "$phase" == "Error" ]]; then
      break
    fi
    sleep 0.$(printf "%03d" "$POLL_MS")
  done
  t2=$(date +%s%3N)

  printf "%s,%s,%s,%s,%s,%s\n" "$i" "$id" "$((t1-t0))" "$((t2-t0))" "$phase" "$polls" >> "$CLIENT_CSV"

  curl -sS -X DELETE "$ORCH_URL/api/v1/sandboxes/$id" >/dev/null || true
  sleep 0.2
  echo "[$i/$ITER] id=$id phase=$phase ready_ms=$((t2-t0))"
done

tr -d '\000' < "$LOG_FILE" | jq -c --arg p "$PREFIX" 'select(.msg=="perf.provision_sandbox" and (.sandbox|startswith($p)))' > "$PROV_JSONL"
tr -d '\000' < "$LOG_FILE" | jq -c --arg p "$PREFIX" 'select(.msg=="perf.stage" and (.sandbox|startswith($p)))' > "$STAGE_JSONL"

metric_file() { echo "$OUT_DIR/metric_$1.txt"; }
collect_metric() {
  local name=$1
  local jqexpr=$2
  jq -r "$jqexpr // empty" "$PROV_JSONL" > "$(metric_file "$name")"
}
collect_stage_metric() {
  local stage=$1
  jq -r --arg s "$stage" 'select(.stage==$s) | .duration' "$STAGE_JSONL" > "$(metric_file "stage_${stage//./_}")"
}

# client ready ms
awk -F, 'NR>1{print $4+0}' "$CLIENT_CSV" > "$(metric_file client_ready_ms)"

# provision metrics (ns)
collect_metric total '.total'
collect_metric pod_sandbox_create_total '.pod_sandbox_create_total'
collect_metric container_image_pull '.container_image_pull'
collect_metric container_create_start_total '.container_create_start_total'
collect_metric network_policy_apply '.network_policy_apply'
collect_metric wait_sandbox_ready '.wait_sandbox_ready'
collect_metric hostport_publish_apply '.hostport_publish_apply'
collect_metric wait_published_tcp_ready '.wait_published_tcp_ready'
collect_metric state_refresh_and_save_running '.state_refresh_and_save_running'

# stage metrics (ns)
collect_stage_metric pod.aggregate_resources
collect_stage_metric pod.ensure_parent_cgroup
collect_stage_metric pod.run_pod_sandbox
collect_stage_metric pod.enforce_cgroup_limits
collect_stage_metric pod.status_query
collect_stage_metric pod.create_total
collect_stage_metric container.ensure_tmpfs_mount
collect_stage_metric container.create
collect_stage_metric container.start
collect_stage_metric container.enforce_cgroup_limits
collect_stage_metric container.status_query
collect_stage_metric container.create_start_total

stat_line() {
  local metric=$1
  local unit=$2
  local file=$(metric_file "$metric")
  if [[ ! -s "$file" ]]; then
    printf "| %s | 0 | - | - | - | - | - |\n" "$metric"
    return
  fi
  awk -v unit="$unit" '
    {v[NR]=$1; sum+=$1}
    END{
      n=NR
      if(n==0){exit}
      asort(v)
      p50=v[int((n-1)*0.50)+1]
      p95=v[int((n-1)*0.95)+1]
      p99=v[int((n-1)*0.99)+1]
      mean=sum/n
      min=v[1]; max=v[n]
      # convert ns->ms when needed
      if (unit=="ns") {mean/=1000000; p50/=1000000; p95/=1000000; p99/=1000000; min/=1000000; max/=1000000}
      printf "| %s | %d | %.3f | %.3f | %.3f | %.3f | %.3f |\n", METRIC, n, mean, p50, p95, p99, max
    }
  ' METRIC="$metric" "$file"
}

{
  echo "# Nginx Sandbox Provisioning Perf ($PREFIX)"
  echo
  echo "- iterations: $ITER"
  echo "- orch url: $ORCH_URL"
  echo "- sbxlet log file: $LOG_FILE"
  echo
  echo "## Client-Observed"
  echo "| metric | n | mean_ms | p50_ms | p95_ms | p99_ms | max_ms |"
  echo "|---|---:|---:|---:|---:|---:|---:|"
  stat_line client_ready_ms ms
  echo
  echo "## sbxlet perf.provision_sandbox (ns->ms)"
  echo "| metric | n | mean_ms | p50_ms | p95_ms | p99_ms | max_ms |"
  echo "|---|---:|---:|---:|---:|---:|---:|"
  stat_line total ns
  stat_line pod_sandbox_create_total ns
  stat_line container_image_pull ns
  stat_line container_create_start_total ns
  stat_line network_policy_apply ns
  stat_line wait_sandbox_ready ns
  stat_line hostport_publish_apply ns
  stat_line wait_published_tcp_ready ns
  stat_line state_refresh_and_save_running ns
  echo
  echo "## sbxlet perf.stage (ns->ms)"
  echo "| metric | n | mean_ms | p50_ms | p95_ms | p99_ms | max_ms |"
  echo "|---|---:|---:|---:|---:|---:|---:|"
  stat_line stage_pod_aggregate_resources ns
  stat_line stage_pod_ensure_parent_cgroup ns
  stat_line stage_pod_run_pod_sandbox ns
  stat_line stage_pod_enforce_cgroup_limits ns
  stat_line stage_pod_status_query ns
  stat_line stage_pod_create_total ns
  stat_line stage_container_ensure_tmpfs_mount ns
  stat_line stage_container_create ns
  stat_line stage_container_start ns
  stat_line stage_container_enforce_cgroup_limits ns
  stat_line stage_container_status_query ns
  stat_line stage_container_create_start_total ns
} > "$SUMMARY_MD"

echo "done: $OUT_DIR"
