# Go-IM 六边形架构重构说明

## 📋 架构概述

项目已成功重构为六边形架构（Hexagonal Architecture），也称为端口适配器架构（Ports and Adapters）。这种架构模式提供了清晰的分层结构，增强了代码的可测试性、可维护性和扩展性。

## 🏗️ 架构分层

### 1. 领域层 (Domain Layer)
**位置**: `internal/domain/`

**职责**: 包含核心业务逻辑，不依赖任何外部技术

#### 📁 实体 (Entities)
- `entities/user.go` - 用户领域实体
- `entities/message.go` - 消息领域实体  
- `entities/group.go` - 群组和群组成员实体

#### 📁 值对象 (Value Objects)
- `valueobjects/conversation_type.go` - 会话类型值对象
- `valueobjects/member_role.go` - 成员角色值对象

#### 📁 领域服务 (Domain Services)
- 包含跨实体的业务逻辑（待扩展）

### 2. 应用层 (Application Layer)  
**位置**: `internal/application/`

**职责**: 编排业务用例，定义端口接口

#### 📁 端口 (Ports)
- `ports/repositories.go` - 数据仓储端口接口
- `ports/services.go` - 外部服务端口接口

#### 📁 用例 (Use Cases)
- `usecases/user_usecase.go` - 用户相关用例
- `usecases/message_usecase.go` - 消息相关用例

### 3. 基础设施层 (Infrastructure Layer)
**位置**: `internal/infrastructure/`

**职责**: 实现端口接口，提供技术实现

#### 📁 持久化适配器
- `adapters/persistence/user_repository.go` - 用户仓储实现

#### 📁 外部服务适配器  
- `adapters/external/id_generator.go` - ID生成器实现
- `adapters/external/password_service.go` - 密码服务实现

### 4. 表示层 (Presentation Layer)
**位置**: `internal/presentation/`

**职责**: 处理外部请求，适配不同协议

#### 📁 HTTP适配器
- `http/user_handler.go` - 用户HTTP处理器

#### 📁 WebSocket适配器
- 待实现：WebSocket消息处理器

## 🔄 依赖关系

```
表示层 → 应用层 → 领域层
     ↙         ↙
基础设施层 → 应用层端口
```

### 依赖倒置原则
- **领域层**: 零外部依赖，纯业务逻辑
- **应用层**: 只依赖领域层，通过端口定义外部需求
- **基础设施层**: 实现应用层端口，提供技术实现
- **表示层**: 依赖应用层用例，处理外部交互

## 🚀 启动方式

### 原有架构
```bash
go run cmd/server/main.go
```

### 六边形架构 
```bash
go run cmd/hexagonal_server/main.go
```

## 📊 架构优势

### ✅ 可测试性
- 业务逻辑与技术实现分离
- 通过模拟接口轻松进行单元测试
- 用例层可独立测试

### ✅ 可维护性  
- 清晰的职责分离
- 高内聚，低耦合
- 易于理解和修改

### ✅ 可扩展性
- 新增功能只需实现相应端口
- 技术栈更换不影响业务逻辑
- 支持多种协议和存储方式

### ✅ 业务优先
- 领域模型驱动设计
- 业务规则集中管理
- 技术细节外置

## 🔧 核心组件

### 端口接口

#### 数据仓储端口
```go
type UserRepository interface {
    Save(ctx context.Context, user *entities.User) error
    GetByID(ctx context.Context, id string) (*entities.User, error)
    GetByUsername(ctx context.Context, username string) (*entities.User, error)
    // ...更多方法
}
```

#### 外部服务端口
```go  
type PasswordService interface {
    HashPassword(password string) (string, error)
    VerifyPassword(hashedPassword, password string) bool
}

type IDGenerator interface {
    GenerateUserID() string
    GenerateMessageID() string
    // ...更多方法
}
```

### 用例编排

```go
type UserUseCase struct {
    userRepo        ports.UserRepository
    passwordSvc     ports.PasswordService
    authSvc         ports.AuthService
    // ...更多依赖
}

func (uc *UserUseCase) Register(ctx context.Context, req *RegisterRequest) (*RegisterResponse, error) {
    // 1. 验证业务规则
    // 2. 创建领域实体  
    // 3. 调用仓储保存
    // 4. 返回结果
}
```

### 依赖注入

```go
func main() {
    // 1. 基础设施层适配器
    userRepo := persistence.NewUserRepositoryAdapter(db)
    passwordSvc := external.NewPasswordServiceAdapter()
    
    // 2. 应用层用例
    userUseCase := usecases.NewUserUseCase(userRepo, passwordSvc, ...)
    
    // 3. 表示层处理器
    userHandler := http.NewUserHandler(userUseCase)
    
    // 4. 路由配置
    setupRoutes(r, userHandler)
}
```

## 📝 实现特点

### 🎯 领域驱动设计
- 用户实体封装业务行为
- 值对象确保数据一致性
- 业务规则在领域层集中管理

### 🔌 插件式架构
- 存储可切换：MySQL、MongoDB、内存等
- 协议可扩展：HTTP、WebSocket、gRPC等  
- 服务可替换：认证、缓存、消息队列等

### 🧪 测试友好
- Mock接口轻松测试用例
- 业务逻辑无外部依赖
- 集成测试通过替换适配器实现

## 🔮 扩展方向

### 短期扩展
1. **完善消息用例**: 实现消息发送、撤回等完整功能
2. **WebSocket适配器**: 支持实时消息推送
3. **群组管理用例**: 群组创建、成员管理等
4. **文件上传用例**: 文件存储和管理

### 中期扩展  
1. **事件驱动**: 引入领域事件，支持异步处理
2. **CQRS模式**: 读写分离，优化查询性能
3. **多租户支持**: 企业级多租户架构
4. **微服务拆分**: 按业务域拆分独立服务

### 长期演进
1. **Event Sourcing**: 事件溯源，完整业务轨迹
2. **DDD聚合**: 复杂领域建模
3. **分布式事务**: Saga模式处理跨服务事务
4. **云原生架构**: 容器化、服务网格等

## 📚 相关资源

- [六边形架构原理](https://alistair.cockburn.us/hexagonal-architecture/)
- [领域驱动设计](https://domainlanguage.com/ddd/)
- [清洁架构](https://blog.cleancoder.com/uncle-bob/2012/08/13/the-clean-architecture.html)

## 🤝 开发规范

### 新增功能流程
1. **分析业务需求** → 确定涉及的领域概念
2. **设计领域模型** → 创建/扩展实体和值对象  
3. **定义端口接口** → 抽象外部依赖
4. **实现用例逻辑** → 编排业务流程
5. **创建适配器** → 实现技术细节
6. **添加处理器** → 暴露外部接口

### 代码规范
- 领域层禁止引入外部依赖
- 接口定义要简洁明确
- 用例方法要原子化
- 错误处理要统一
- 日志记录要结构化

---

**重构完成！** 🎉

项目现在具备了更好的架构基础，为后续功能扩展和技术演进奠定了坚实基础。 