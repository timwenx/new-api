#!/usr/bin/env bash

set -Eeuo pipefail
umask 077

SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
REPO_DIR=${NEW_API_REPO_DIR:-$(cd -- "$SCRIPT_DIR/.." && pwd)}
COMPOSE_FILE=${NEW_API_COMPOSE_FILE:-$REPO_DIR/docker-compose.yml}
BACKUP_DIR=${NEW_API_BACKUP_DIR:-$REPO_DIR/backups}
STATE_FILE=${NEW_API_DEPLOY_STATE_FILE:-$BACKUP_DIR/last-deploy-state}
BUILD_STATE_FILE=${NEW_API_BUILD_STATE_FILE:-/run/new-api-deploy-build.state}

SERVICE_NAME=${NEW_API_SERVICE_NAME:-new-api}
MYSQL_CONTAINER=${NEW_API_MYSQL_CONTAINER:-new-api-mysql}
REDIS_CONTAINER=${NEW_API_REDIS_CONTAINER:-new-api-redis}
CANARY_CONTAINER=${NEW_API_CANARY_CONTAINER:-new-api-deploy-canary}
CANARY_VOLUME=${NEW_API_CANARY_VOLUME:-new-api-deploy-canary-data}
PRODUCTION_PORT=${NEW_API_PRODUCTION_PORT:-3000}
CANARY_PORT=${NEW_API_CANARY_PORT:-3001}
REQUIRED_ENV_KEYS=${NEW_API_REQUIRED_ENV_KEYS:-SQL_DSN,REDIS_CONN_STRING,SESSION_SECRET,CRYPTO_SECRET}

log() {
  printf '[new-api-deploy] %s\n' "$*"
}

die() {
  printf '[new-api-deploy] ERROR: %s\n' "$*" >&2
  exit 1
}

usage() {
  cat <<'EOF'
用法：sudo ./bin/deploy-local-image.sh <命令>

命令：
  preflight     只读检查 Git、Compose、环境变量、容器和生产健康状态
  build-start   在 systemd 后台启动当前 Git HEAD 的 Docker 镜像构建
  build-status  查看最近一次后台构建状态和日志；成功时校验镜像 revision
  canary        在 127.0.0.1:3001 启动隔离 SQLite canary，并完成真实 WS 101 握手
  backup        备份 MySQL、Compose 和当前镜像，记录本次发布状态
  deploy        仅重建 new-api 服务；失败时自动恢复旧 Compose 和旧镜像
  verify        连续检查生产 HTTP、镜像、revision、重启数以及 MySQL/Redis ID
  cleanup       删除 canary 容器及其独立数据卷
  rollback      使用最近一次 backup 的 Compose 恢复应用镜像，不自动恢复数据库
  help          显示本说明

默认发布镜像：local/new-api:git-<当前提交前 8 位>
如需覆盖，可设置 TARGET_IMAGE，例如：
  sudo TARGET_IMAGE=local/new-api:manual-test ./bin/deploy-local-image.sh canary

推荐顺序：
  preflight -> build-start -> build-status -> canary -> backup -> deploy -> verify -> cleanup
EOF
}

require_root() {
  [[ ${EUID:-$(id -u)} -eq 0 ]] || die '请使用 sudo 运行该命令'
}

require_commands() {
  local command_name
  for command_name in "$@"; do
    command -v "$command_name" >/dev/null 2>&1 || die "缺少命令：$command_name"
  done
}

git_revision() {
  git -C "$REPO_DIR" rev-parse HEAD
}

short_revision() {
  git_revision | cut -c1-8
}

target_image() {
  if [[ -n ${TARGET_IMAGE:-} ]]; then
    printf '%s\n' "$TARGET_IMAGE"
  else
    printf 'local/new-api:git-%s\n' "$(short_revision)"
  fi
}

validate_image_name() {
  [[ $1 =~ ^[A-Za-z0-9][A-Za-z0-9._/:@-]*$ ]] || die "无效镜像名称：$1"
}

compose() {
  (
    cd "$REPO_DIR"
    docker compose -f "$COMPOSE_FILE" "$@"
  )
}

container_id() {
  docker inspect "$1" --format '{{.Id}}'
}

container_image_id() {
  docker inspect "$1" --format '{{.Image}}'
}

http_code() {
  local port=$1
  local path=$2
  curl -sS -o /dev/null -w '%{http_code}' --max-time 3 "http://127.0.0.1:${port}${path}" || true
}

wait_for_health() {
  local port=$1
  local attempts=${2:-60}
  local attempt code

  for ((attempt = 1; attempt <= attempts; attempt++)); do
    code=$(http_code "$port" '/api/status')
    if [[ $code == 200 ]]; then
      log "端口 $port 在第 $attempt 次检查恢复健康"
      return 0
    fi
    sleep 1
  done
  return 1
}

state_value() {
  local key=$1
  awk -F= -v key="$key" '$1 == key { sub(/^[^=]*=/, ""); print; exit }' "$STATE_FILE"
}

require_state() {
  [[ -s $STATE_FILE ]] || die "找不到发布状态：$STATE_FILE；请先运行 backup"
}

assert_target_image() {
  local image=$1
  local expected_revision=$2
  local actual_revision

  docker image inspect "$image" >/dev/null 2>&1 || die "镜像不存在：$image"
  actual_revision=$(docker image inspect "$image" --format '{{index .Config.Labels "org.opencontainers.image.revision"}}')
  [[ $actual_revision == "$expected_revision" ]] || die "镜像 revision 不匹配：期望 $expected_revision，实际 $actual_revision"
}

check_worktree() {
  local line path
  local unexpected=0

  while IFS= read -r line; do
    [[ -z $line ]] && continue
    path=${line:3}
    case "$path" in
      docker-compose.yml | backups | backups/*)
        ;;
      *)
        printf '[new-api-deploy] 非预期工作区修改：%s\n' "$line" >&2
        unexpected=1
        ;;
    esac
  done < <(git -C "$REPO_DIR" status --porcelain --untracked-files=all)

  [[ $unexpected -eq 0 ]] || die '请先处理上述代码修改；生产 Compose 和 backups 目录除外'
}

check_compose_environment() {
  compose config --format json | python3 -c '
import json
import sys

service_name = sys.argv[1]
required = [item for item in sys.argv[2].split(",") if item]
config = json.load(sys.stdin)
services = config.get("services", {})
if service_name not in services:
    raise SystemExit(f"Compose 中不存在服务：{service_name}")
environment = services[service_name].get("environment") or {}
missing = [key for key in required if not str(environment.get(key, ""))]
if missing:
    raise SystemExit("Compose 必需环境变量为空：" + ",".join(missing))
' "$SERVICE_NAME" "$REQUIRED_ENV_KEYS"
}

check_auto_update_cron() {
  local count
  count=$({ crontab -l 2>/dev/null || true; } | python3 -c '
import sys

count = 0
for line in sys.stdin:
    stripped = line.strip().lower()
    if not stripped or stripped.startswith("#"):
        continue
    if any(term in stripped for term in (
        "/root/new-api",
        "calciumion/new-api",
        "quantumnous/new-api",
        "timwenx/new-api",
    )):
        count += 1
print(count)
')
  [[ $count == 0 ]] || die "检测到 $count 条可能覆盖本地镜像的 new-api 定时任务"
}

preflight() {
  require_root
  require_commands docker git curl gzip openssl python3 awk grep install mktemp systemd-run systemctl journalctl
  [[ -f $COMPOSE_FILE ]] || die "Compose 文件不存在：$COMPOSE_FILE"
  [[ -f $REPO_DIR/Dockerfile ]] || die "Dockerfile 不存在：$REPO_DIR/Dockerfile"
  [[ -f $REPO_DIR/.dockerignore ]] || die '缺少 .dockerignore'
  grep -qxF '.env' "$REPO_DIR/.dockerignore" || die '.dockerignore 未排除 .env'
  grep -qxF '/backups' "$REPO_DIR/.dockerignore" || die '.dockerignore 未排除 backups'
  grep -qxF '/docker-compose.yml' "$REPO_DIR/.dockerignore" || die '.dockerignore 未排除生产 Compose'

  [[ $(git -C "$REPO_DIR" branch --show-current) == main ]] || die '生产构建必须从 main 分支执行'
  check_worktree
  docker info >/dev/null
  check_compose_environment
  check_auto_update_cron

  local container
  for container in "$SERVICE_NAME" "$MYSQL_CONTAINER" "$REDIS_CONTAINER"; do
    [[ $(docker inspect "$container" --format '{{.State.Running}}' 2>/dev/null || true) == true ]] || die "容器未运行：$container"
  done
  [[ $(http_code "$PRODUCTION_PORT" '/api/status') == 200 ]] || die "生产端口 $PRODUCTION_PORT 不健康"

  log "预检通过：branch=main revision=$(git_revision) target=$(target_image)"
}

build_start() {
  preflight

  local revision short image unit docker_path previous_unit
  revision=$(git_revision)
  short=${revision:0:8}
  image=$(target_image)
  validate_image_name "$image"
  if [[ -s $BUILD_STATE_FILE ]]; then
    previous_unit=$(awk -F= '$1 == "UNIT" { print $2 }' "$BUILD_STATE_FILE")
    if [[ -n $previous_unit ]] && systemctl is-active --quiet "$previous_unit"; then
      die "已有构建正在运行：$previous_unit"
    fi
  fi
  unit="new-api-build-${short}-$(date -u +%H%M%S)-${RANDOM}"
  docker_path=$(command -v docker)

  systemd-run \
    --unit="$unit" \
    --description="Build new-api image $image" \
    --property=Type=exec \
    --setenv=DOCKER_BUILDKIT=1 \
    "$docker_path" build \
    --label "org.opencontainers.image.revision=$revision" \
    --label 'org.opencontainers.image.source=https://github.com/timwenx/new-api' \
    -t "$image" \
    "$REPO_DIR"

  printf 'UNIT=%s\nIMAGE=%s\nREVISION=%s\n' "$unit" "$image" "$revision" > "$BUILD_STATE_FILE"
  chmod 600 "$BUILD_STATE_FILE"
  log "构建已启动：unit=$unit image=$image"
  log '使用 build-status 查看进度；构建完成前不要执行 canary'
}

build_status() {
  require_root
  require_commands docker systemctl journalctl awk
  [[ -s $BUILD_STATE_FILE ]] || die "找不到构建状态：$BUILD_STATE_FILE；请先运行 build-start"

  local unit image revision active result exit_status actual_revision
  unit=$(awk -F= '$1 == "UNIT" { print $2 }' "$BUILD_STATE_FILE")
  image=$(awk -F= '$1 == "IMAGE" { sub(/^[^=]*=/, ""); print }' "$BUILD_STATE_FILE")
  revision=$(awk -F= '$1 == "REVISION" { print $2 }' "$BUILD_STATE_FILE")
  active=$(systemctl show "$unit" -p ActiveState --value)
  result=$(systemctl show "$unit" -p Result --value)
  exit_status=$(systemctl show "$unit" -p ExecMainStatus --value)

  printf 'unit=%s active=%s result=%s exit_status=%s image=%s\n' "$unit" "$active" "$result" "$exit_status" "$image"
  journalctl -u "$unit" --no-pager -n 30 -o cat

  if [[ $active == active || $active == activating ]]; then
    log '构建仍在进行中'
    return 0
  fi
  [[ $result == success && $exit_status == 0 ]] || die '构建失败，请查看上方日志'
  docker image inspect "$image" >/dev/null 2>&1 || die "构建结束但镜像不存在：$image"
  actual_revision=$(docker image inspect "$image" --format '{{index .Config.Labels "org.opencontainers.image.revision"}}')
  [[ $actual_revision == "$revision" ]] || die "镜像 revision 不匹配：$actual_revision"
  log "构建成功：image=$image revision=$revision"
}

canary() {
  require_root
  require_commands docker curl openssl python3

  local image revision
  image=$(target_image)
  revision=$(git_revision)
  validate_image_name "$image"
  assert_target_image "$image" "$revision"

  if docker container inspect "$CANARY_CONTAINER" >/dev/null 2>&1; then
    docker rm -f "$CANARY_CONTAINER" >/dev/null
  fi
  if docker volume inspect "$CANARY_VOLUME" >/dev/null 2>&1; then
    docker volume rm "$CANARY_VOLUME" >/dev/null
  fi

  docker volume create "$CANARY_VOLUME" >/dev/null
  docker run -d \
    --name "$CANARY_CONTAINER" \
    --restart no \
    -p "127.0.0.1:${CANARY_PORT}:3000" \
    -e TZ=Asia/Shanghai \
    -v "$CANARY_VOLUME:/data" \
    "$image" >/dev/null

  wait_for_health "$CANARY_PORT" 30 || die 'canary 未在 30 秒内恢复健康'
  [[ $(http_code "$CANARY_PORT" '/v1/responses') == 401 ]] || die 'canary /v1/responses 未认证检查失败'
  [[ $(http_code "$CANARY_PORT" '/v1/realtime') == 401 ]] || die 'canary /v1/realtime 未认证检查失败'

  (
    set -Eeuo pipefail
    local temp_dir password setup_response setup_success login_response login_success user_id
    local add_response add_success token_id token ws_status
    temp_dir=$(mktemp -d)
    trap 'rm -rf "$temp_dir"' EXIT
    password=$(openssl rand -hex 18)

    printf '{"username":"canaryroot","password":"%s","confirmPassword":"%s","SelfUseModeEnabled":true,"DemoSiteEnabled":false}' \
      "$password" "$password" > "$temp_dir/setup.json"
    setup_response=$(curl -fsS --max-time 10 \
      -H 'Content-Type: application/json' \
      --data-binary @"$temp_dir/setup.json" \
      "http://127.0.0.1:${CANARY_PORT}/api/setup")
    setup_success=$(printf '%s' "$setup_response" | python3 -c 'import json,sys; print(str(json.load(sys.stdin).get("success", False)).lower())')
    [[ $setup_success == true ]] || die 'canary 初始化失败'

    printf '{"username":"canaryroot","password":"%s"}' "$password" > "$temp_dir/login.json"
    login_response=$(curl -fsS --max-time 10 \
      -c "$temp_dir/cookie" \
      -H 'Content-Type: application/json' \
      --data-binary @"$temp_dir/login.json" \
      "http://127.0.0.1:${CANARY_PORT}/api/user/login")
    login_success=$(printf '%s' "$login_response" | python3 -c 'import json,sys; print(str(json.load(sys.stdin).get("success", False)).lower())')
    [[ $login_success == true ]] || die 'canary 登录失败'
    user_id=$(printf '%s' "$login_response" | python3 -c 'import json,sys; print(json.load(sys.stdin)["data"]["id"])')

    add_response=$(curl -fsS --max-time 10 \
      -b "$temp_dir/cookie" \
      -H "New-Api-User: $user_id" \
      -H 'Content-Type: application/json' \
      --data '{"name":"ws-canary","expired_time":-1,"unlimited_quota":true}' \
      "http://127.0.0.1:${CANARY_PORT}/api/token/")
    add_success=$(printf '%s' "$add_response" | python3 -c 'import json,sys; print(str(json.load(sys.stdin).get("success", False)).lower())')
    [[ $add_success == true ]] || die 'canary 临时令牌创建失败'

    curl -fsS --max-time 10 \
      -b "$temp_dir/cookie" \
      -H "New-Api-User: $user_id" \
      "http://127.0.0.1:${CANARY_PORT}/api/token/?p=0&page_size=10" > "$temp_dir/tokens.json"
    token_id=$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["data"]["items"][0]["id"])' "$temp_dir/tokens.json")

    curl -fsS --max-time 10 -X POST \
      -b "$temp_dir/cookie" \
      -H "New-Api-User: $user_id" \
      "http://127.0.0.1:${CANARY_PORT}/api/token/${token_id}/key" > "$temp_dir/token-key.json"
    token=$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["data"]["key"])' "$temp_dir/token-key.json")

    ws_status=$(CANARY_TOKEN="$token" CANARY_PORT="$CANARY_PORT" python3 <<'PY'
import base64
import hashlib
import os
import socket
import struct

host = "127.0.0.1"
port = int(os.environ["CANARY_PORT"])
key = base64.b64encode(os.urandom(16)).decode()
token = os.environ["CANARY_TOKEN"]
request = (
    "GET /v1/responses HTTP/1.1\r\n"
    f"Host: {host}:{port}\r\n"
    "Upgrade: websocket\r\n"
    "Connection: Upgrade\r\n"
    f"Sec-WebSocket-Key: {key}\r\n"
    "Sec-WebSocket-Version: 13\r\n"
    "Sec-WebSocket-Protocol: responses\r\n"
    f"Authorization: Bearer {token}\r\n"
    "\r\n"
).encode()

with socket.create_connection((host, port), timeout=5) as sock:
    sock.sendall(request)
    response = b""
    while b"\r\n\r\n" not in response:
        chunk = sock.recv(4096)
        if not chunk:
            break
        response += chunk

    header_text = response.split(b"\r\n\r\n", 1)[0].decode("latin1")
    lines = header_text.split("\r\n")
    status = lines[0]
    headers = {}
    for line in lines[1:]:
        name, separator, value = line.partition(":")
        if separator:
            headers[name.lower()] = value.strip()

    expected_accept = base64.b64encode(
        hashlib.sha1((key + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11").encode()).digest()
    ).decode()
    if status != "HTTP/1.1 101 Switching Protocols":
        raise SystemExit("WebSocket status 不是 101：" + status)
    if headers.get("sec-websocket-accept") != expected_accept:
        raise SystemExit("Sec-WebSocket-Accept 校验失败")
    if headers.get("sec-websocket-protocol") != "responses":
        raise SystemExit("WebSocket 子协议不是 responses")

    payload = struct.pack("!H", 1000)
    mask = os.urandom(4)
    masked_payload = bytes(byte ^ mask[index % 4] for index, byte in enumerate(payload))
    sock.sendall(bytes([0x88, 0x80 | len(payload)]) + mask + masked_payload)
    print("101")
PY
    )
    [[ $ws_status == 101 ]] || die "canary WebSocket 握手失败：$ws_status"
  )

  [[ $(http_code "$PRODUCTION_PORT" '/api/status') == 200 ]] || die 'canary 检查期间生产服务异常'
  [[ $(docker inspect "$CANARY_CONTAINER" --format '{{.RestartCount}}') == 0 ]] || die 'canary 发生重启'
  log "canary 通过：HTTP=200 ResponsesWS=101 production=200 image=$image"
}

backup() {
  preflight

  local image revision short stamp database_backup database_temp compose_backup
  local old_image rollback_tag mysql_id redis_id state_temp
  image=$(target_image)
  revision=$(git_revision)
  short=${revision:0:8}
  validate_image_name "$image"
  assert_target_image "$image" "$revision"

  mkdir -p "$BACKUP_DIR"
  stamp=$(date -u +%Y%m%dT%H%M%SZ)
  database_backup="$BACKUP_DIR/new-api-before-${short}-${stamp}.sql.gz"
  database_temp=$(mktemp "/tmp/new-api-before-${short}.XXXXXX.sql.gz")
  compose_backup="$BACKUP_DIR/docker-compose-before-${short}-${stamp}.yml"

  if ! docker exec "$MYSQL_CONTAINER" sh -c \
    'MYSQL_PWD="$MYSQL_ROOT_PASSWORD" exec mysqldump -uroot --single-transaction --quick --routines --triggers "$MYSQL_DATABASE"' \
    | gzip -c > "$database_temp"; then
    rm -f "$database_temp"
    die 'MySQL 备份失败'
  fi
  gzip -t "$database_temp"
  install -m 600 "$database_temp" "$database_backup"
  rm -f "$database_temp"

  install -m 600 "$COMPOSE_FILE" "$compose_backup"
  old_image=$(container_image_id "$SERVICE_NAME")
  rollback_tag="local/new-api:rollback-before-${short}-${stamp}"
  docker tag "$old_image" "$rollback_tag"
  mysql_id=$(container_id "$MYSQL_CONTAINER")
  redis_id=$(container_id "$REDIS_CONTAINER")

  state_temp=$(mktemp "$BACKUP_DIR/.deploy-state.XXXXXX")
  printf '%s\n' \
    "TARGET_IMAGE=$image" \
    "SOURCE_REVISION=$revision" \
    "OLD_IMAGE_ID=$old_image" \
    "ROLLBACK_TAG=$rollback_tag" \
    "COMPOSE_BACKUP=$compose_backup" \
    "DATABASE_BACKUP=$database_backup" \
    "MYSQL_ID=$mysql_id" \
    "REDIS_ID=$redis_id" > "$state_temp"
  chmod 600 "$state_temp"
  mv "$state_temp" "$STATE_FILE"

  log "备份完成：database=$database_backup"
  log "Compose 备份：$compose_backup"
  log "旧镜像回滚标签：$rollback_tag"
}

restore_previous_compose() {
  local compose_backup=$1
  [[ -f $compose_backup ]] || die "Compose 备份不存在：$compose_backup"
  install -m 600 "$compose_backup" "$COMPOSE_FILE"
  compose up -d --no-deps "$SERVICE_NAME"
  wait_for_health "$PRODUCTION_PORT" 60 || die '应用回滚后仍未恢复健康'
}

deploy() {
  require_root
  require_commands docker curl awk python3 chmod chown mktemp
  require_state

  local image revision compose_backup mysql_id redis_id temp_compose
  image=$(state_value TARGET_IMAGE)
  revision=$(state_value SOURCE_REVISION)
  compose_backup=$(state_value COMPOSE_BACKUP)
  mysql_id=$(state_value MYSQL_ID)
  redis_id=$(state_value REDIS_ID)
  validate_image_name "$image"
  assert_target_image "$image" "$revision"
  [[ $(container_id "$MYSQL_CONTAINER") == "$mysql_id" ]] || die 'MySQL 容器在 backup 后发生变化'
  [[ $(container_id "$REDIS_CONTAINER") == "$redis_id" ]] || die 'Redis 容器在 backup 后发生变化'
  [[ $(http_code "$PRODUCTION_PORT" '/api/status') == 200 ]] || die '切换前生产服务不健康'

  temp_compose=$(mktemp "$REPO_DIR/.docker-compose.deploy.XXXXXX")
  if ! awk -v service="$SERVICE_NAME" -v image="$image" '
    BEGIN { in_services = 0; in_target = 0; replaced = 0 }
    $0 == "services:" { in_services = 1 }
    in_services && $0 ~ /^  [^[:space:]][^:]*:[[:space:]]*$/ {
      name = $0
      sub(/^  /, "", name)
      sub(/:[[:space:]]*$/, "", name)
      in_target = (name == service)
    }
    in_target && $0 ~ /^    image:[[:space:]]*/ {
      print "    image: " image
      replaced++
      next
    }
    { print }
    END { if (replaced != 1) exit 42 }
  ' "$COMPOSE_FILE" > "$temp_compose"; then
    rm -f "$temp_compose"
    die '无法唯一定位 Compose 中 new-api 的 image 字段'
  fi

  if ! (
    cd "$REPO_DIR"
    docker compose -f "$temp_compose" config --quiet
  ); then
    rm -f "$temp_compose"
    die '修改后的 Compose 校验失败'
  fi
  chmod --reference="$COMPOSE_FILE" "$temp_compose"
  chown --reference="$COMPOSE_FILE" "$temp_compose"
  mv "$temp_compose" "$COMPOSE_FILE"

  compose up -d --no-deps "$SERVICE_NAME"
  if ! wait_for_health "$PRODUCTION_PORT" 60; then
    log '新镜像健康检查失败，开始自动回滚'
    restore_previous_compose "$compose_backup"
    die '新镜像部署失败，已恢复旧应用镜像'
  fi

  [[ $(container_id "$MYSQL_CONTAINER") == "$mysql_id" ]] || die '部署过程中 MySQL 容器 ID 发生变化'
  [[ $(container_id "$REDIS_CONTAINER") == "$redis_id" ]] || die '部署过程中 Redis 容器 ID 发生变化'
  log "生产切换完成：image=$image"
}

verify() {
  require_root
  require_commands docker curl python3 grep
  require_state

  local image revision mysql_id redis_id expected_image_id actual_image_id actual_revision
  local ok=0 sample code root_code setup_code responses_code realtime_code restart_count configured_image fatal_count
  image=$(state_value TARGET_IMAGE)
  revision=$(state_value SOURCE_REVISION)
  mysql_id=$(state_value MYSQL_ID)
  redis_id=$(state_value REDIS_ID)
  expected_image_id=$(docker image inspect "$image" --format '{{.Id}}')

  for ((sample = 1; sample <= 15; sample++)); do
    code=$(http_code "$PRODUCTION_PORT" '/api/status')
    [[ $code == 200 ]] && ((ok += 1))
    sleep 1
  done
  [[ $ok -eq 15 ]] || die "生产健康检查仅通过 $ok/15"

  root_code=$(http_code "$PRODUCTION_PORT" '/')
  setup_code=$(http_code "$PRODUCTION_PORT" '/api/setup')
  responses_code=$(http_code "$PRODUCTION_PORT" '/v1/responses')
  realtime_code=$(http_code "$PRODUCTION_PORT" '/v1/realtime')
  [[ $root_code == 200 && $setup_code == 200 ]] || die "页面或 setup 异常：root=$root_code setup=$setup_code"
  [[ $responses_code == 401 && $realtime_code == 401 ]] || die "未认证路由异常：responses=$responses_code realtime=$realtime_code"

  actual_image_id=$(container_image_id "$SERVICE_NAME")
  [[ $actual_image_id == "$expected_image_id" ]] || die '生产容器未使用目标镜像'
  actual_revision=$(docker image inspect "$actual_image_id" --format '{{index .Config.Labels "org.opencontainers.image.revision"}}')
  [[ $actual_revision == "$revision" ]] || die "生产镜像 revision 不匹配：$actual_revision"
  restart_count=$(docker inspect "$SERVICE_NAME" --format '{{.RestartCount}}')
  [[ $restart_count == 0 ]] || die "生产容器重启次数为 $restart_count"
  [[ $(container_id "$MYSQL_CONTAINER") == "$mysql_id" ]] || die 'MySQL 容器 ID 已变化'
  [[ $(container_id "$REDIS_CONTAINER") == "$redis_id" ]] || die 'Redis 容器 ID 已变化'

  configured_image=$(compose config --format json | python3 -c '
import json
import sys
print(json.load(sys.stdin)["services"][sys.argv[1]]["image"])
' "$SERVICE_NAME")
  [[ $configured_image == "$image" ]] || die "Compose 镜像不是目标值：$configured_image"
  fatal_count=$(docker logs --since 10m "$SERVICE_NAME" 2>&1 | grep -Eic 'panic|fatal' || true)
  [[ $fatal_count == 0 ]] || die "最近日志包含 $fatal_count 条 panic/fatal"

  log "生产验证通过：health=15/15 image=$image revision=$revision restart=0"
  log "路由状态：root=200 setup=200 responses_unauth=401 realtime_unauth=401"
}

cleanup() {
  require_root
  require_commands docker
  if docker container inspect "$CANARY_CONTAINER" >/dev/null 2>&1; then
    docker rm -f "$CANARY_CONTAINER" >/dev/null
  fi
  if docker volume inspect "$CANARY_VOLUME" >/dev/null 2>&1; then
    docker volume rm "$CANARY_VOLUME" >/dev/null
  fi
  log 'canary 容器和独立数据卷已清理；发布备份与回滚状态保留'
}

rollback() {
  require_root
  require_commands docker curl install
  require_state

  local compose_backup old_image mysql_id redis_id
  compose_backup=$(state_value COMPOSE_BACKUP)
  old_image=$(state_value OLD_IMAGE_ID)
  mysql_id=$(state_value MYSQL_ID)
  redis_id=$(state_value REDIS_ID)
  restore_previous_compose "$compose_backup"

  [[ $(container_image_id "$SERVICE_NAME") == "$old_image" ]] || die '回滚后生产容器镜像不符合记录'
  [[ $(container_id "$MYSQL_CONTAINER") == "$mysql_id" ]] || die '回滚过程中 MySQL 容器 ID 发生变化'
  [[ $(container_id "$REDIS_CONTAINER") == "$redis_id" ]] || die '回滚过程中 Redis 容器 ID 发生变化'
  log "应用镜像已回滚；数据库未自动恢复，备份位置：$(state_value DATABASE_BACKUP)"
}

main() {
  case ${1:-help} in
    preflight)
      preflight
      ;;
    build-start)
      build_start
      ;;
    build-status)
      build_status
      ;;
    canary)
      canary
      ;;
    backup)
      backup
      ;;
    deploy)
      deploy
      ;;
    verify)
      verify
      ;;
    cleanup)
      cleanup
      ;;
    rollback)
      rollback
      ;;
    help | --help | -h)
      usage
      ;;
    *)
      usage >&2
      die "未知命令：$1"
      ;;
  esac
}

main "$@"
