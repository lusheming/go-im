### 多实例部署指南（Docker Compose + Nginx 负载均衡）

本指南提供一键脚本与 Compose 覆盖文件，支持 `app` 多实例横向扩展，并通过 Nginx 统一对外暴露端口，完整支持 WebSocket。

---

## 前置条件
- Docker ≥ 20.10
- Docker Compose v2（`docker compose version`）
- 端口占用：主机 `8080` 可用（对外暴露）

## 目录结构
- `docker/docker-compose.yml`：基础编排（app/mysql/redis）
- `docker/docker-compose.multi.yml`：多实例覆盖（去除 app 端口，新增 Nginx）
- `docker/nginx/nginx.conf`：Nginx 负载均衡（含 WebSocket 升级）
- `docker/deploy_multi_instance.sh`：一键部署脚本
- `docker/scale.sh`：快速扩缩容脚本
- `docker/config.yml`：部署时挂载到容器 `/app/config.yml`（可选，亦可用环境变量覆盖）

## 快速开始
```bash
# 授权脚本
chmod +x docker/deploy_multi_instance.sh docker/scale.sh

# 一键部署（默认副本数 3，可通过 APP_REPLICAS 覆盖）
APP_REPLICAS=3 ./docker/deploy_multi_instance.sh

# 访问
open http://localhost:8080/ui
open http://localhost:8080/app
```

## 扩缩容
```bash
# 将副本数扩到 5
./docker/scale.sh 5

# 查看服务状态
docker compose -f docker/docker-compose.yml -f docker/docker-compose.multi.yml ps

# 查看负载均衡与应用日志
docker compose -f docker/docker-compose.yml -f docker/docker-compose.multi.yml logs -f nginx
docker compose -f docker/docker-compose.yml -f docker/docker-compose.multi.yml logs -f app
```

## WebSocket 支持
- 外部接入：`ws://localhost:8080/ws`
- Nginx 已配置：
  - `Upgrade/Connection` 头透传
  - `proxy_read_timeout` / `proxy_send_timeout` 足够大（长连接）
  - DNS 解析 `app` 服务名至多容器 IP（`resolver 127.0.0.11` + `server app:8080 resolve`）

## 配置与密钥
- 配置优先级：默认 < `config.yml` < 环境变量
- 集群需保证：`JWT_SECRET`、对象存储回调域名等在所有实例一致
- 可直接编辑 `docker/config.yml` 或在 Compose 中通过 `environment` 覆盖

## 健康检查
- 健康接口：`GET /healthz`
- 脚本会等待 `http://localhost:8080/healthz` 就绪后再退出

## 常见问题排查
- 8080 被占用：修改 `docker/docker-compose.multi.yml` 中 `nginx.ports` 端口映射
- `config.yml` 挂载错误：确保存在 `docker/config.yml`（脚本会自动创建或从根目录复制）
- 只有 1 个实例在对外端口：多实例访问必须通过 Nginx，对外只暴露 Nginx（`app` 不再直接映射主机端口）
- WebSocket 404：确保访问路径是 `/ws`（非根路径）
- 群消息收不到：客户端需先发送 `subscribe_group` 消息进行订阅

## 性能与容量建议
- 按连接数/CPU/内存评估每实例承载量，使用 `./docker/scale.sh N` 水平扩展
- 调整限流：`WSSendQPS`、`WSSendBurst`（Compose 环境变量覆盖）
- Redis 建议使用哨兵/集群；消息库生产建议 TiDB 或 MySQL 主从
- 群大规模 fanout：启用 Kafka 并运行 `cmd/group_consumer`

## 生产增强项（可选）
- Nginx TLS：在 `nginx.conf` 中开启 `listen 443 ssl` 并挂载证书
- 日志/监控：抓取 `/metrics`，接入 Prometheus + Grafana
- 优雅滚动：K8s 部署（Deployment + Service + Ingress），配置 Liveness/Readiness

---

## 进阶：K8s（简要示例）
如需直接使用 K8s，可参考以下思路（示例不含完整清单）：
- `Deployment`：`app` 副本 N，设置资源/探针
- `Service`：ClusterIP，`ws` 与 HTTP 共用 8080
- `Ingress`：配置 WebSocket 透传（`Upgrade/Connection` 头）
- Redis/MySQL/TiDB/Kafka 使用托管或独立 Stateful 服务

> 如需完整 K8s YAML，请告知所需组件（含 Redis/MySQL/TiDB/Kafka）与命名空间规划，我会补全交付。 