# go-backend-kit

`go-backend-kit` 是一个确定性 Go 后台脚手架：用严格 YAML 定义资源，即可生成 Echo v5 + GORM 的五接口 CRUD、PostgreSQL 版本化迁移、OpenAPI 3.1、内嵌 Swagger UI 和契约测试。

[English](README.md)

## 环境要求

- Go 1.26.4 或更高版本（Atlas v1.2.3 的最低要求）；推荐 Go 1.26.5 或更新的受支持补丁版本
- Docker，用于 PostgreSQL 开发环境和 Atlas 迁移

## 快速开始

```bash
go install github.com/alphayan/go-backend-kit/cmd/gobackend@v0.1.0
gobackend new product-api --module github.com/yourname/product-api
cd product-api
```

创建 `product.yaml`：

```yaml
schema_version: 1
name: Product
table: products
route: /products
fields:
  - name: name
    type: string
    required: true
    max_length: 120
    searchable: true
    unique: true
  - name: status
    type: string
    required: true
    enum: [enabled, disabled]
    filterable: true
  - name: owner_id
    type: int64
    filterable: true
    sortable: true
```

生成代码、创建迁移并启动：

```bash
go tool gobackend add product.yaml
docker compose up -d postgres
make migration name=create_products
DATABASE_URL='postgres://app:app@localhost:5432/app?sslmode=disable' make migrate-apply
DATABASE_URL='postgres://app:app@localhost:5432/app?sslmode=disable' make run
```

访问 `http://localhost:8080/docs`。

## 命令

```text
gobackend new <dir> --module <module-path>
go tool gobackend add <resource.yaml>
go tool gobackend generate
go tool gobackend check
go tool gobackend version
```

生成项目使用 Go 的 `tool` 指令固定 `gobackend` 和 GORM CLI。Atlas 官方已不再维护可由 Go 安装的当前 CLI 包，因此本地和 CI 都通过 `arigaio/atlas:1.2.3-community` 固定开源 Atlas CLI；Atlas Go 引擎和 GORM Provider 仍固定在 `go.mod`。

Community 配置仅用于生成和应用版本化迁移，以及比较已应用数据库与生成的 GORM schema；本项目不把高级迁移 lint、回滚、迁移测试、审批策略或高级数据库对象治理视为 Community 能力。应用前必须审查每一份生成的 SQL 迁移。

## 五个接口

```text
GET    /api/v1/products
GET    /api/v1/products/:id
POST   /api/v1/products
PATCH  /api/v1/products/:id
DELETE /api/v1/products/:id
```

列表支持 `page`、`page_size`、`sort`、`q` 和声明过的精确筛选。PATCH 字段区分“未提供、显式 null、实际值”三种状态，能正确更新 `false`、`0` 和空字符串。

成功响应统一为 `{"data": ...}`，错误响应包含稳定的 `code`、安全的 `message`、可选 `details` 和 `request_id`。模型固定包含 `id int64`、`created_at`、`updated_at`，输入 DTO 无法写入这些字段。时间统一按 UTC 生成并以 RFC3339 输出，v0.1.0 固定硬删除。

## YAML Schema v1

支持 `string`、`text`、`bool`、`int32`、`int64`、`float64`、`decimal`、`time`、`uuid`、`json`。

`decimal` 使用 JSON 字符串传输，避免精度损失。

字段属性支持 `required`、`nullable`、`default`、`unique`、`index`、`enum`、`min`、`max`、`max_length`、`searchable`、`filterable`、`sortable`。未知键、危险名称或路由、重复基础字段、默认值类型错误、互相矛盾的约束都会在替换任何生成文件前报错。

首版不生成关联。`user_id` 等业务 ID 作为普通标量字段声明；领域扩展直接写在普通手写 `.go` 文件中，生成器永不覆盖。

## 生成结构与安全边界

每个资源是一个领域内聚包：DTO 负责输入验证和值转换，具体 store 负责 GORM 数据访问，HTTP handler 负责协议边界和短链路编排。代码不生成 repository/service 接口、通用泛型仓储或依赖注入链。固定版本的官方 GORM CLI 继续生成筛选与排序使用的字段辅助。

只有带受认可生成标记的文件会被管理；普通手写 `.go` 文件会被保留。生成过程先进入临时目录，完成模板渲染、`go/format` 与 OpenAPI 校验后，再逐文件原子替换。连续生成两次无差异；`check` 会发现缺失、陈旧或被修改的生成文件。

默认包含 request ID、JSON `slog`、panic recover、1 MiB body 限制、15 秒请求超时、安全头、可配置 CORS、标准 `http.Server` 超时和 10 秒优雅停机，并提供 `/health/live`、`/health/ready`、`/openapi.json`、`/docs`。

PostgreSQL 连接池默认最多 25 个连接、25 个空闲连接，连接最长存活 30 分钟、最长空闲 5 分钟；可通过 `DB_MAX_OPEN_CONNS`、`DB_MAX_IDLE_CONNS`、`DB_CONN_MAX_LIFETIME` 和 `DB_CONN_MAX_IDLE_TIME` 覆盖。启动时会在 `DB_CONNECT_TIMEOUT`（默认 5 秒）内执行数据库探测，失败则拒绝启动；停机时显式关闭连接池。

生产启动绝不调用 `AutoMigrate`。SQLite 的 `AutoMigrate` 只用于本地契约快测；CI 先把经过审查的 Atlas SQL 迁移应用到 PostgreSQL，再复跑相同契约。

## 测试

```bash
go test -race ./...
go vet ./...
go tool govulncheck ./...
```

端到端测试会执行新建项目、添加多个资源、重新生成、漂移检查、所有字段类型编译和五接口契约。

## v0.1.0 不包含

登录、JWT、RBAC、软删除、关联建模、MySQL、Redis、任务队列、文件上传和管理端前端。

## License

MIT
