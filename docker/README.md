# Docker 部署

## 目录结构
- docker/Dockerfile: 构建镜像
- docker/docker-compose.yml: 一键启动 app + MySQL + Redis

## 启动
```bash
cd docker
# 构建并启动
docker compose up -d --build
# 查看日志
docker compose logs -f app
```

启动后：
- 应用: http://localhost:8080
  - 测试页: /ui
  - Web 客户端: /app
- MySQL: 127.0.0.1:3306 (root/example)
- Redis: 127.0.0.1:6379

## 配置覆盖
- 环境变量在 docker-compose.yml 中已示例，可按需覆盖
- 可提供外部 config.yml（已挂载到 /app/config.yml），会被读取并可被环境变量覆盖

## 数据持久化
- MySQL 数据: docker 卷 mysql_data
- Redis 数据: docker 卷 redis_data
- 上传文件: docker 卷 uploads（映射到容器 /app/uploads）

## 生产建议
- 替换 IM_JWT_SECRET
- 如需启用 OSS 直传，设置 IM_OSS_ENABLED=true 并提供 AK/SK、Bucket、Endpoint
- 大群建议启用 Kafka（此 compose 未包含，可自行扩展） 