# Go-IM 管理后台

基于 Vue 3 + Element Plus 的现代化 IM 系统管理界面。

## 功能特性

### 🎛️ 仪表盘
- 系统概览（用户数、在线数、群组数、消息数）
- 最近活动记录
- 实时统计数据

### 👥 用户管理
- 用户列表查看
- 在线状态显示
- 用户详情查看
- 用户禁用操作

### 👫 群组管理
- 群组列表查看
- 成员数量统计
- 群组详情查看
- 群组解散操作

### 📊 消息统计
- 按日期范围查询
- 今日/本周/本月消息量
- 消息趋势分析

### ⚙️ 系统设置
- 系统名称配置
- 最大群成员数设置
- 消息保留天数配置
- 注册功能开关

## 技术栈

- **前端框架**: Vue 3 (Composition API)
- **UI 组件**: Element Plus
- **构建方式**: CDN 引入 (免构建)
- **状态管理**: Vue 3 Reactivity
- **HTTP 客户端**: Fetch API

## 快速开始

### 1. 创建管理员账户
首先需要注册一个用户名为 `admin` 的账户：

```bash
# 启动服务
go run cmd/server/main.go

# 或使用 make
make run
```

然后访问用户注册页面创建 admin 账户：
- 访问 `http://localhost:8080/app`
- 注册用户名为 `admin` 的账户

### 2. 访问管理后台
- 访问 `http://localhost:8080/admin`
- 使用 admin 账户登录

### 3. 管理后台功能
登录后即可使用所有管理功能：
- 查看系统统计
- 管理用户和群组
- 查看消息统计
- 配置系统设置

## API 接口

管理后台调用的 API 接口：

```
POST   /api/admin/login              # 管理员登录
GET    /api/admin/stats              # 系统统计
GET    /api/admin/users              # 用户列表
POST   /api/admin/users/:id/ban      # 禁用用户
GET    /api/admin/groups             # 群组列表
POST   /api/admin/groups/:id/disband # 解散群组
GET    /api/admin/message-stats      # 消息统计
GET    /api/admin/activities         # 最近活动
GET    /api/admin/settings           # 系统设置
PUT    /api/admin/settings           # 保存设置
```

## 开发说明

### 目录结构
```
web/admin/
├── index.html          # 主页面
├── main.js             # Vue 应用逻辑
└── README.md           # 说明文档
```

### 扩展功能
如需添加新功能：

1. **前端**: 在 `main.js` 中添加新的 API 调用和页面逻辑
2. **后端**: 在 `cmd/server/main.go` 的 adminGroup 中添加新的路由
3. **权限**: 所有管理 API 都需要 admin 用户权限

### 自定义样式
可以在 `index.html` 的 `<style>` 标签中修改样式，或者添加新的 CSS 文件。

## 安全注意事项

1. **管理员权限**: 只有用户名为 `admin` 的用户才能访问管理后台
2. **Token 验证**: 所有管理 API 都需要有效的 JWT Token
3. **HTTPS**: 生产环境建议启用 HTTPS
4. **访问控制**: 可考虑添加 IP 白名单等额外安全措施

## 常见问题

### Q: 无法登录管理后台？
A: 确保已注册用户名为 `admin` 的账户，并使用正确的密码。

### Q: 页面显示异常？
A: 检查网络连接，确保能正常访问 CDN 资源（Vue 3、Element Plus）。

### Q: 数据不更新？
A: 点击对应页面的"刷新"按钮，或者重新切换标签页。

### Q: 如何添加新的管理功能？
A: 参考现有代码结构，在前端添加 UI 和 API 调用，在后端添加对应的路由处理。 