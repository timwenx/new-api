# 本地镜像分步发布流程

本流程用于在生产服务器上，从 `/root/new-api` 当前 `main` 提交构建本地 Docker 镜像，先通过 3001 端口验证，再仅替换 3000 端口的 `new-api` 容器。

脚本位置：`bin/deploy-local-image.sh`

## 安全边界

- 只允许从 `main` 构建。
- 允许生产环境保留本地 `docker-compose.yml` 和 `backups/`；发现其他未提交代码时停止。
- `.env`、Compose 和备份必须已被 `.dockerignore` 排除，避免进入镜像构建上下文。
- canary 使用独立 SQLite 数据卷，只绑定 `127.0.0.1:3001`，不连接生产 MySQL 或 Redis。
- 生产切换固定执行 `docker compose up -d --no-deps new-api`。
- 切换前记录 MySQL、Redis 容器 ID；部署和回滚后必须保持不变。
- 新镜像健康检查失败时自动恢复切换前的 Compose。
- 数据库只自动备份，不自动恢复。需要恢复数据库时必须人工确认，避免覆盖上线后的新数据。
- 脚本不会拉取 Git、合并分支或删除旧备份。

## 默认环境

| 项目 | 默认值 |
| --- | --- |
| 源码目录 | `/root/new-api` |
| Compose | `/root/new-api/docker-compose.yml` |
| 生产服务 | `new-api` |
| MySQL 容器 | `new-api-mysql` |
| Redis 容器 | `new-api-redis` |
| 生产端口 | `3000` |
| canary 端口 | `3001` |
| 镜像标签 | `local/new-api:git-<Git SHA 前 8 位>` |
| 备份目录 | `/root/new-api/backups` |

必要时可通过脚本开头对应的环境变量覆盖这些值。最常用的是 `TARGET_IMAGE`，但正常发布建议使用默认的 Git SHA 标签。如果覆盖镜像标签，应在 `build-start`、`canary` 和 `backup` 阶段持续使用同一个值。

## 0. 更新并检查源码

先确认 GitHub 上的目标改动已经进入 `main`，再在服务器执行：

```bash
cd /root/new-api
sudo git fetch origin main --prune
sudo git merge --ff-only origin/main
sudo git status --short --branch
```

预期只允许出现生产 Compose 和备份目录：

```text
 M docker-compose.yml
?? backups/
```

## 1. 预检

```bash
cd /root/new-api
sudo ./bin/deploy-local-image.sh preflight
```

该步骤不修改任何服务，会检查：

- 当前分支和工作区；
- Docker 与 Compose；
- 生产 `.env` 中必需变量是否非空，但不会打印变量值；
- new-api、MySQL、Redis 是否运行；
- 3000 端口是否健康；
- 是否残留可能覆盖本地镜像的 new-api 定时更新任务。

## 2. 后台构建镜像

```bash
sudo ./bin/deploy-local-image.sh build-start
```

构建由 systemd transient unit 执行，SSH 断开不会终止构建。重复运行下面命令查看进度：

```bash
sudo ./bin/deploy-local-image.sh build-status
```

必须看到“构建成功”，并确认镜像 revision 与当前 Git HEAD 一致后再继续。

## 3. 运行 3001 canary

```bash
sudo ./bin/deploy-local-image.sh canary
```

该步骤会重新创建脚本专用的 canary 容器和数据卷，并验证：

- `/api/status` 返回 200；
- 未认证的 `/v1/responses` 和 `/v1/realtime` 返回 401；
- 使用一次性 canary 用户和令牌完成 `/v1/responses` 的真实 WebSocket 101 握手；
- 生产 3000 在 canary 测试期间仍返回 200；
- canary 重启次数为 0。

## 4. 创建上线前备份

```bash
sudo ./bin/deploy-local-image.sh backup
```

该步骤会创建：

- MySQL 压缩备份，并执行 `gzip -t`；
- 当前生产 Compose 备份；
- 当前生产镜像的时间戳回滚标签；
- `/root/new-api/backups/last-deploy-state`，记录目标镜像、revision 和依赖容器 ID。

只有 `backup` 成功后才能执行 `deploy`。

## 5. 切换生产 new-api

```bash
sudo ./bin/deploy-local-image.sh deploy
```

脚本只更新 Compose 中 `new-api` 服务的 `image` 字段，验证 Compose 后执行：

```bash
docker compose up -d --no-deps new-api
```

如果 60 秒内未恢复健康，脚本会自动恢复备份 Compose 并重新启动旧应用镜像。MySQL 和 Redis 不会被主动重建。

## 6. 生产验证

```bash
sudo ./bin/deploy-local-image.sh verify
```

验证内容包括：

- 15 次连续健康检查全部为 200；
- 首页和 `/api/setup` 为 200；
- 未认证的 Responses/Realtime 路由为 401；
- 生产容器镜像 ID、revision 与目标一致；
- 重启次数为 0；
- MySQL、Redis 容器 ID 与备份前一致；
- 最近 10 分钟无 `panic` 或 `fatal` 日志。

## 7. 清理 canary

```bash
sudo ./bin/deploy-local-image.sh cleanup
```

该步骤只删除 canary 容器和独立数据卷，数据库备份、Compose 备份和回滚状态会保留。

## 应用回滚

如果上线后需要恢复上一版应用：

```bash
sudo ./bin/deploy-local-image.sh rollback
```

`rollback` 会检查旧镜像、3000 健康状态以及 MySQL/Redis 容器 ID。它只恢复应用 Compose 和旧镜像，不会自动导入数据库备份。

## 完整命令清单

```bash
cd /root/new-api
sudo ./bin/deploy-local-image.sh preflight
sudo ./bin/deploy-local-image.sh build-start
sudo ./bin/deploy-local-image.sh build-status
sudo ./bin/deploy-local-image.sh canary
sudo ./bin/deploy-local-image.sh backup
sudo ./bin/deploy-local-image.sh deploy
sudo ./bin/deploy-local-image.sh verify
sudo ./bin/deploy-local-image.sh cleanup
```
