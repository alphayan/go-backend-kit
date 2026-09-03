# go-backend-kit

`go-backend-kit` 是一个确定性 Go 后台脚手架：用严格 YAML 定义资源，即可生成五接口 CRUD、版本化迁移、OpenAPI 3.1、内嵌 Swagger UI 和契约测试。Session 项目还会生成由 Go 二进制内嵌提供的 Vue 管理端。

默认 `gobackend new` 选择是 Echo + SQLite + slog，不包含 Redis、NATS 或认证。PostgreSQL、Fiber、Redis、NATS、zap/zerolog、JWT 校验以及 Echo 数据库 Session 在创建项目时按需选择。Provider 在生成期静态编入；生成的二进制没有插件注册表或 DI 容器。

[English](README.md)

默认档是刻意保持轻量的个人档：Echo + SQLite + slog，不包含 Redis、NATS、认证或运维栈。PostgreSQL、Fiber、Redis、NATS、其他日志后端、JWT 校验、Echo 数据库 Session 和生产档都在创建项目时按需选择。

## 环境要求

- Go 1.27.1 或更高版本；生成项目固定 `go 1.27.1`
- Session 前端开发需要 Node.js 24.20.0 LTS 和 pnpm 11.25.0；生成项目会固定两个版本，但生产运行时不依赖它们。
- 默认 SQLite 项目运行应用不需要 Docker。选择 PostgreSQL、Redis、NATS 或执行 Atlas 迁移时需要 Docker。
- 生产档还会使用 Docker 启动本地应用、Prometheus 和 Grafana。

## 快速开始

```bash
go install github.com/alphayan/go-backend-kit/cmd/gobackend@v0.2.0
gobackend new product-api --module github.com/yourname/product-api
cd product-api
```

`new` 的可选参数（括号内为默认值）：

```text
--http echo|fiber           (默认 echo)
--database sqlite|postgres  (个人档默认 sqlite；生产档默认 postgres)
--cache none|redis          (默认 none)
--messaging none|nats       (默认 none)
--logging slog|zap|zerolog  (默认 slog)
--auth none|jwt|session     (默认 none；session 仅支持 Echo)
--profile personal|production (默认 personal)
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
make migration name=create_products
make migrate-apply
make run
```

默认 SQLite 项目把数据放在 `data/app.db`，不需要 Compose。PostgreSQL 项目仍使用 `docker compose up -d postgres` 和 `DATABASE_URL`。

访问 `http://localhost:8080/docs`。

## 可选生产档

只有在项目需要更完整的本地运维闭环时才选择这个档：

    gobackend new product-api --module github.com/yourname/product-api --profile production
    cd product-api
    cp .env.example .env
    cp .env.postgres.example .env.postgres
    chmod 600 .env .env.postgres
    # 按生成项目 docs/postgres-operations.md 填写凭据、初始化数据库并应用迁移
    make up

它会增加固定版本的应用 Dockerfile、Compose 一键启动、容器健康检查、可配置的 JSON 日志级别和服务元数据、Prometheus 指标/告警以及基础 Grafana 看板。日常可使用 make logs、make down、make compose-config。当前刻意不生成 Kubernetes 清单，也不生成 CI 镜像发布/部署任务。

生产档未显式指定数据库时使用 PostgreSQL；刻意单进程项目仍可显式选择 `--database sqlite`。生产 PostgreSQL 会分离初始化、迁移和运行角色，将数据库管理秘密与 API 环境分开，并提供原生备份/新库恢复脚本；已有卷不会自动改造。异地备份目标、实际切换和 TLS 部署需要单独配置验证。

## 命令

```text
gobackend new <dir> --module <module-path> [--http echo|fiber] [--database sqlite|postgres] [--cache none|redis] [--messaging none|nats] [--logging slog|zap|zerolog] [--auth none|jwt|session] [--profile personal|production]
go tool gobackend add <resource.yaml>
go tool gobackend generate
go tool gobackend check
go tool gobackend version
```

生成项目使用 Go 的 `tool` 指令固定 `gobackend` 和 GORM CLI。`go tool gobackend generate` 保留官方 `go tool gorm gen` 工作流；`gormgen/query_gen.go` 只由官方 GORM CLI 生成，gobackend 不重写其输出。Atlas 官方已不再维护可由 Go 安装的当前 CLI 包，因此本地和 CI 都通过 `arigaio/atlas:1.3.0-community` 固定开源 Atlas CLI；Atlas Go 引擎和 GORM Provider 仍固定在 `go.mod`。

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

项目根目录的 `.gobackend-generated.json` 记录生成器实际拥有的路径及其 SHA-256 摘要；普通手写 `.go` 文件和无关的官方 GORM 输出都会被保留。清理陈旧文件时，只有当前内容仍与清单摘要完全一致才会删除；一旦检测到人工修改，生成会报错停止，不会删除该文件。生成过程先进入临时目录，完成模板渲染、`go/format` 与 OpenAPI 校验后，再逐文件原子替换。连续生成两次无差异；`check` 会发现缺失、陈旧或被修改的生成文件。

默认包含 request ID、JSON `slog`、panic recover、1 MiB body 限制、15 秒请求超时、安全头、可配置 CORS、标准 `http.Server` 超时和 10 秒优雅停机，并提供 `/health/live`、`/health/ready`、`/openapi.json`、`/docs`。

PostgreSQL 连接池默认最多 25 个连接、25 个空闲连接，连接最长存活 30 分钟、最长空闲 5 分钟；可通过 `DB_MAX_OPEN_CONNS`、`DB_MAX_IDLE_CONNS`、`DB_CONN_MAX_LIFETIME` 和 `DB_CONN_MAX_IDLE_TIME` 覆盖。启动时会在 `DB_CONNECT_TIMEOUT`（默认 5 秒）内执行数据库探测，失败则拒绝启动；停机时按逆序关闭已选择的客户端。

生产启动绝不调用 `AutoMigrate`。SQLite 的 `AutoMigrate` 只用于本地契约快测；CI 先把经过审查的 Atlas SQL 迁移应用到 PostgreSQL，再复跑相同契约。

个人档保持生成运行时轻量。生产档额外提供 /metrics，记录 HTTP 请求量/耗时/并发数和数据库连接池指标，并在本地预置 Prometheus 与 Grafana。profile 在创建项目后不可就地切换。

## 升级已生成的项目

在项目目录中运行新版 `gobackend upgrade` 默认只预览，`gobackend upgrade --apply` 才应用。新项目的 `.gobackend-scaffold.json` 记录原始脚手架摘要，应提交到版本控制；普通 `generate` 不会刷新它或接管用户修改。`add` 引发的模块整理只刷新整理前仍匹配原始摘要的 `go.mod`／`go.sum`。

没有该基线的旧项目必须提供 `--baseline /absolute/pristine-old-project`：用原始版本生成器、相同 module/provider 和资源定义重建并核验的原始项目，或可靠的原始快照。不要把当前已修改项目或新版候选目录当作旧基线。升级要求已有生成文件清单，不会猜测文件归属；不支持就地切换 provider/profile。

预览会下载所需 Go 模块，并在 gitignore 的 `.gobackend/upgrade-*` 中留下 `candidate/` 与 `plan.json`；不写业务源文件。应用前将所有被替换/删除的原文件连同权限备份到该目录的 `before/`。用户与上游同时改过的文件会阻止整批应用；手工对照候选文件合并后，用 `--keep internal/app/app.go` 等精确路径确认保留合并结果并推进其上游基线。`--keep` 只接受脚手架路径，不接受生成文件、`.env`、迁移或资源定义。自定义依赖同样需要人工合并 `go.mod`／`go.sum`，不能直接保留旧版本依赖后声称升级完成。

应用使用项目锁、写前校验和逐文件替换；可观察写入失败会回滚已写文件，不覆盖回滚期间出现的新修改。它不是整个目录的原子事务：断电/强制退出时须根据 `plan.json` 和 `before/` 核验恢复或重跑；新增文件没有旧备份，人工回退前核对候选内容再删除。不要在应用期间编辑、运行其他生成器或部署该目录。保留恢复目录至验收结束，再按精确路径清理。升级不会读取真实 `.env`、执行数据库迁移、Git 提交或部署。

升级后执行 `go mod tidy`、`go tool gobackend generate`、`go tool gobackend check`、`go test -race ./...`、`go vet ./...` 和 `go tool govulncheck ./...`；Session 项目还需冻结 pnpm 安装、前端构建/浏览器测试。数据库变更仍需单独审查迁移并验证恢复，不能用代码升级代替生产切换。

## Session 认证选项

`--auth session` 是仅限 Echo 的数据库登录套件，包含可撤销 HttpOnly Cookie、Argon2id 密码、固定 `admin`/`viewer` RBAC、全局标准库跨源保护、有界登录/KDF 限流和尽力写入的审计日志。它新增 `POST /auth/login`、`POST /auth/logout`、`GET /auth/me`、`POST /auth/password`，并为资源生成路由权限表和内嵌 Vue 管理端。

Session 项目会把 `auth_users`、`auth_sessions`、`auth_audit_logs` 模型加入 Atlas 目标 schema；PostgreSQL 还包含共享限流表 `auth_rate_limits`。不会附带手写认证表 SQL，也不会在运行时调用 `AutoMigrate`。启动前必须显式生成、审查并应用迁移。空用户表要求成对设置引导邮箱和密码。生产必须使用 `__Host-session` Secure Cookie、TLS 和 HSTS。PostgreSQL 的 IP／邮箱额度由数据库短事务跨实例共享；SQLite 限流仅适用于单进程。可信代理还必须正确配置 Echo 的 IP 提取。审计在业务提交后写入，因此进程在窄窗口崩溃可能丢失该事件。

管理端位于 `/admin/`，按资源生成带 Zod 校验的类型化表单、搜索/筛选/排序表格和分页，并支持管理员增删改、viewer 只读、登录/注销、密码修改、亮色/暗色/跟随系统主题、响应式布局和可访问对话框。可运行 `make frontend-install`、`make frontend-typecheck`、`make frontend-test`、`make frontend-build`；`make frontend-dev` 通过同源代理启动 Vite。生产 Docker 构建会先编译前端，再由 Go 二进制内嵌 `web/dist`，运行镜像不包含 Node.js 或 pnpm。

## 测试

Session 管理端还包含仅管理员可见的用户管理和审计查询：创建账号、调整角色归属、禁用/启用、会话列表与撤销、分页审计筛选。禁止修改自身角色/状态，修改他人访问权限会撤销其会话。审计保留期由 `AUTH_AUDIT_RETENTION_DAYS` 明确开启（默认 `0`，不自动删除）。

```bash
go test -race ./...
go vet ./...
go tool govulncheck ./...
./scripts/frontend-e2e.sh
```

端到端测试会执行新建项目、添加多个资源、重新生成、漂移检查、所有字段类型编译和五接口契约。

Session 生成项目包含串行/并行口令基准 `BenchmarkPassword`。可复现命令、十次重复样本、内存分配和本机 RSS 边界见 [口令基准记录](docs/password-benchmark-2026-09-04.md)；不将微基准视为生产登录延迟或安全强弱证明。

`scripts/frontend-e2e.sh` 需要 Docker 执行显式 SQLite 迁移，并安装固定版本 Chromium。除冻结锁文件、类型检查、辅助函数单测、构建/嵌入检查外，还会实际操作浏览器验证登录、用户/会话管理、审计筛选、密码修改、资源 CRUD/搜索/筛选/分页和 viewer 权限。数据仅位于临时生成项目中；失败时保留目录供排查。

`scripts/frontend-e2e.sh production` 生成生产档、显式选择 SQLite 作为隔离测试库，并增加仅测试用的 Go 标准库 TLS 代理。Chromium 的 `.test` 域名只在该浏览器进程映射到回环地址；验证 HTTP 拒收 Secure Cookie、HTTPS 接受 `__Host-session`/HttpOnly/Path/SameSite 属性、同源资源写入、跨站表单 POST 拒绝和 Lax 顶层 GET 放行，以及轮换/注销后的旧 Cookie 重放拒绝。代理单测验证转发头防伪并保留原始 Host/Origin/Cookie。默认使用 4187–4189 端口（可通过 `E2E_PORT` 选连续三个端口），测试关闭自有服务，绝不复用已有服务。

该代理只复制进临时项目，不是生产部署产物；浏览器仅在测试上下文忽略 httptest 自签名证书错误，不修改系统信任库。HSTS 检查仅证明代理响应头，不证明真实证书签发/续期或浏览器持久化强制升级。实际域名、反代 IP 信任配置、生产容量和异地备份仍需独立验收；PostgreSQL 迁移与共享限流使用各自的集成测试，不由这套 SQLite TLS 测试代替。

## 当前不包含

已创建项目中更换 provider、第二种 ORM、刷新令牌、可在线编辑的角色定义、密码重置/MFA/SSO、软删除、关联建模、MySQL、自动 CRUD 缓存、NATS/JetStream 拓扑和文件上传。

## License

MIT
