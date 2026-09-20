#!/usr/bin/env sh
set -eu

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
scratch="$repo_root/.codex_tmp/bin"
task_temp="$repo_root/.codex_tmp/temp"
mkdir -p "$scratch" "$task_temp"
export TMPDIR="$task_temp"
cd "$repo_root"

if find . -type f \( -name '*.key' -o -name '*.pfx' -o -name '*.p12' -o -name '*.pem' -o -name '*.sqlite' -o -name '*.sqlite-*' -o -name '*.sqlite3' -o -name '*.sqlite3-*' -o -name '*.db' -o -name '*.db-*' \) \
  -not -path './.git/*' -not -path './.codex_tmp/*' | grep -q .; then
  echo 'Forbidden private-data file found.' >&2
  exit 1
fi

go run ./scripts/governancecheck

unformatted=$(gofmt -l cmd internal pkg scripts/governancecheck)
if [ -n "$unformatted" ]; then
  echo "Unformatted Go files:" >&2
  echo "$unformatted" >&2
  exit 1
fi

go test ./...
go vet ./...
go build -o "$scratch/pfremote" ./cmd/pfremote
go build -o "$scratch/pfremote-mcp" ./cmd/pfremote-mcp
go build -o "$scratch/pfremote-migrate" ./cmd/pfremote-migrate
go build -o "$scratch/pfremote-recovery" ./cmd/pfremote-recovery
go build -o "$scratch/pfremoted" ./cmd/pfremoted
go build -o "$scratch/pfremote-gateway" ./cmd/pfremote-gateway

"$scratch/pfremote-migrate" plan --input fixtures/migration/legacy-inventory-v1.synthetic.json >"$task_temp/migration-plan-a.json"
"$scratch/pfremote-migrate" plan --input fixtures/migration/legacy-inventory-v1.synthetic.json >"$task_temp/migration-plan-b.json"
cmp "$task_temp/migration-plan-a.json" "$task_temp/migration-plan-b.json"
"$scratch/pfremote-migrate" observe \
  --legacy fixtures/migration/observation-legacy-v1.synthetic.json \
  --candidate fixtures/migration/observation-candidate-v1.synthetic.json >"$task_temp/migration-observation-report.json"
grep -q '"overall": "ready"' "$task_temp/migration-observation-report.json"
"$scratch/pfremote-migrate" observe \
  --legacy fixtures/migration/observation-legacy-v1.synthetic.json \
  --candidate fixtures/migration/observation-candidate-v1.synthetic.json \
  --format review --locale zh-CN >"$task_temp/migration-observation-review.md"
grep -q '可以进入受控切换准备' "$task_temp/migration-observation-review.md"
grep -q '没有修改、停止或切换任何服务' "$task_temp/migration-observation-review.md"

smoke_root=$(mktemp -d "$repo_root/.codex_tmp/daemon-smoke.XXXXXX")
daemon_pid=''
cleanup_daemon_smoke() {
  if [ -n "$daemon_pid" ] && kill -0 "$daemon_pid" 2>/dev/null; then
    kill "$daemon_pid"
    wait "$daemon_pid" 2>/dev/null || true
  fi
  rm -rf -- "$smoke_root"
}
trap cleanup_daemon_smoke EXIT HUP INT TERM
export XDG_CONFIG_HOME="$smoke_root/config"
export PFREMOTE_LOCAL_ENDPOINT="$smoke_root/daemon-v1.sock"
mkdir -p "$XDG_CONFIG_HOME"
"$scratch/pfremoted" >"$smoke_root/pfremoted.stdout.log" 2>"$smoke_root/pfremoted.stderr.log" &
daemon_pid=$!
attempt=0
while [ ! -S "$PFREMOTE_LOCAL_ENDPOINT" ] && [ "$attempt" -lt 50 ]; do
  if ! kill -0 "$daemon_pid" 2>/dev/null; then
    echo 'PF Remote daemon exited before opening its isolated local endpoint.' >&2
    exit 1
  fi
  sleep 0.1
  attempt=$((attempt + 1))
done
if [ ! -S "$PFREMOTE_LOCAL_ENDPOINT" ]; then
  echo 'PF Remote daemon did not open its isolated local endpoint.' >&2
  exit 1
fi
"$scratch/pfremote" list --json >/dev/null
cleanup_daemon_smoke
trap - EXIT HUP INT TERM

echo 'PF Remote checks passed.'
