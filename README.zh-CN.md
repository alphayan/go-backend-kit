# go-backend-kit

`go-backend-kit` 是一个确定性 Go 后台脚手架：用严格 YAML 定义资源，即可生成 Echo v5 + GORM 的五接口 CRUD、PostgreSQL 版本化迁移、OpenAPI 3.1、内嵌 Swagger UI 和契约测试。

[English](README.md)

## 环境要求

- Go 1.26.5 或更高版本
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

首版不生成关联。`user_id` 等业务 ID 作为普通标量字段声明；领域扩展写在不会被覆盖的 `custom.go` 中。

## 生成结构与安全边界

项目按功能模块组织：Echo 仅在 handler/router 层，service 和 repository 只接收标准 `context.Context`。共享层提供配置、数据库、统一错误与响应、分页、三态字段和 GORM 泛型 CRUD。每个模型的字段辅助由固定版本的官方 GORM CLI 自动生成，并实际用于仓储筛选与排序。

只有带生成标记的文件会被管理。生成过程先进入临时目录，完成模板渲染、`go/format` 与 OpenAPI 校验后，再逐文件原子替换。连续生成两次无差异；`check` 会发现缺失、陈旧或被修改的生成文件。

默认包含 request ID、JSON `slog`、panic recover、1 MiB body 限制、15 秒请求超时、安全头、可配置 CORS、标准 `http.Server` 超时和 10 秒优雅停机，并提供 `/health/live`、`/health/ready`、`/openapi.json`、`/docs`。

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
