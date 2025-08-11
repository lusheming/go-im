#!/bin/bash

# GO-IM 启动脚本
# 使用方法: ./start.sh [dev|prod|docker|stop]

set -e

PROJECT_NAME="go-im"
BINARY_NAME="hexagonal_server"
PID_FILE="/tmp/${PROJECT_NAME}.pid"

# 颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# 检查进程是否运行
is_running() {
    if [ -f "$PID_FILE" ]; then
        local pid=$(cat "$PID_FILE")
        if ps -p "$pid" > /dev/null 2>&1; then
            return 0
        else
            rm -f "$PID_FILE"
            return 1
        fi
    fi
    return 1
}

# 停止服务
stop_service() {
    log_info "正在停止 GO-IM 服务..."
    
    # 停止 Docker 服务
    if docker-compose -f docker/docker-compose.yml ps | grep -q "Up"; then
        log_info "停止 Docker 服务..."
        docker-compose -f docker/docker-compose.yml down
    fi
    
    # 停止本地服务
    if is_running; then
        local pid=$(cat "$PID_FILE")
        kill "$pid"
        rm -f "$PID_FILE"
        log_success "服务已停止 (PID: $pid)"
    else
        # 尝试杀死所有相关进程
        pkill -f "$BINARY_NAME" || true
        log_warn "未找到运行中的服务"
    fi
}

# 编译项目
build_project() {
    log_info "正在编译项目..."
    if go build -o "$BINARY_NAME" cmd/hexagonal_server/main.go; then
        log_success "编译完成"
    else
        log_error "编译失败"
        exit 1
    fi
}

# 启动开发模式
start_dev() {
    log_info "启动开发模式..."
    
    if is_running; then
        log_warn "服务已在运行中，请先停止"
        exit 1
    fi
    
    build_project
    
    log_info "启动服务器..."
    nohup ./"$BINARY_NAME" > "${PROJECT_NAME}.log" 2>&1 &
    local pid=$!
    echo "$pid" > "$PID_FILE"
    
    sleep 2
    if is_running; then
        log_success "服务启动成功 (PID: $pid)"
        log_info "访问地址："
        echo "  - 新 IM 客户端: http://localhost:8080/im"
        echo "  - 测试客户端:   http://localhost:8080/"
        echo "  - WebRTC 测试:  http://localhost:8080/webrtc_test.html"
        echo "  - 管理后台:     http://localhost:8080/admin"
        log_info "日志文件: ${PROJECT_NAME}.log"
    else
        log_error "服务启动失败，请检查日志"
        cat "${PROJECT_NAME}.log"
        exit 1
    fi
}

# 启动生产模式
start_prod() {
    log_info "启动生产模式..."
    
    if is_running; then
        log_warn "服务已在运行中，请先停止"
        exit 1
    fi
    
    # 设置生产环境变量
    export GIN_MODE=release
    export IM_JWT_SECRET="$(openssl rand -base64 32)"
    export IM_ENABLE_METRICS=true
    
    build_project
    
    log_info "启动服务器 (生产模式)..."
    nohup ./"$BINARY_NAME" > "${PROJECT_NAME}.log" 2>&1 &
    local pid=$!
    echo "$pid" > "$PID_FILE"
    
    sleep 2
    if is_running; then
        log_success "生产服务启动成功 (PID: $pid)"
        log_info "访问地址: http://localhost:8080/im"
        log_warn "请修改默认数据库密码和 JWT 密钥"
    else
        log_error "服务启动失败，请检查日志"
        cat "${PROJECT_NAME}.log"
        exit 1
    fi
}

# 启动 Docker 模式
start_docker() {
    log_info "启动 Docker 模式..."
    
    if ! command -v docker &> /dev/null; then
        log_error "Docker 未安装"
        exit 1
    fi
    
    if ! command -v docker-compose &> /dev/null; then
        log_error "Docker Compose 未安装"
        exit 1
    fi
    
    log_info "构建并启动 Docker 服务..."
    docker-compose -f docker/docker-compose.yml up -d --build
    
    log_info "等待服务启动..."
    sleep 10
    
    if docker-compose -f docker/docker-compose.yml ps | grep -q "Up"; then
        log_success "Docker 服务启动成功"
        log_info "访问地址："
        echo "  - IM 客户端: http://localhost:8080/im"
        echo "  - 测试页面: http://localhost:8080/"
        log_info "查看日志: docker-compose -f docker/docker-compose.yml logs -f"
    else
        log_error "Docker 服务启动失败"
        docker-compose -f docker/docker-compose.yml logs
        exit 1
    fi
}

# 显示状态
show_status() {
    log_info "服务状态检查..."
    
    if is_running; then
        local pid=$(cat "$PID_FILE")
        log_success "本地服务运行中 (PID: $pid)"
    else
        log_warn "本地服务未运行"
    fi
    
    if docker-compose -f docker/docker-compose.yml ps | grep -q "Up"; then
        log_success "Docker 服务运行中"
        docker-compose -f docker/docker-compose.yml ps
    else
        log_warn "Docker 服务未运行"
    fi
}

# 显示帮助
show_help() {
    echo "GO-IM 启动脚本"
    echo ""
    echo "使用方法:"
    echo "  $0 [命令]"
    echo ""
    echo "可用命令:"
    echo "  dev     - 启动开发模式 (默认)"
    echo "  prod    - 启动生产模式"
    echo "  docker  - 使用 Docker 启动"
    echo "  stop    - 停止所有服务"
    echo "  restart - 重启服务"
    echo "  status  - 显示服务状态"
    echo "  help    - 显示此帮助信息"
    echo ""
    echo "示例:"
    echo "  $0          # 启动开发模式"
    echo "  $0 dev      # 启动开发模式"
    echo "  $0 docker   # 使用 Docker 启动"
    echo "  $0 restart  # 重启服务"
    echo "  $0 stop     # 停止服务"
}

# 主程序
main() {
    local command=${1:-dev}
    
    case "$command" in
        "dev"|"development")
            start_dev
            ;;
        "prod"|"production")
            start_prod
            ;;
        "docker")
            start_docker
            ;;
        "stop")
            stop_service
            ;;
        "restart")
            log_info "重启服务..."
            stop_service
            sleep 2
            start_dev
            ;;
        "status")
            show_status
            ;;
        "help"|"-h"|"--help")
            show_help
            ;;
        *)
            log_error "未知命令: $command"
            show_help
            exit 1
            ;;
    esac
}

# 检查是否在项目根目录
if [ ! -f "go.mod" ] || [ ! -d "cmd/hexagonal_server" ]; then
    log_error "请在项目根目录运行此脚本"
    exit 1
fi

main "$@"
