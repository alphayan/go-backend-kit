# Session 口令基准记录（2026-09-04）

本记录补原审查 P1-6 的源码、命令、分配量、并发和重复样本证据；同时明确 P1-3 的内存边界。没有修改密码算法或参数，不是一次性能优化前后对比，也不是生产验收。

## 环境与来源

- Apple M2 Ultra，24 个逻辑 CPU，64 GiB 内存；macOS 26.6.2（25G83），darwin/arm64。
- Go 1.27.1；生成模块 `go 1.27.1`；`golang.org/x/crypto v0.56.0`。
- `CGO_ENABLED=1`、`GOTOOLCHAIN=auto`；`GOFLAGS` / `GOEXPERIMENT` 为空；没有设置 `GODEBUG` / `GOFIPS140`。
- 测量时显式设置 `GOGC=100 GOMEMLIMIT=2GiB`，没有 race、coverage、profile 或 trace；未同时运行构建/测试负载，但未停用日常后台服务，未固定 CPU 核或频率。
- 生成器工作树基于 `786f9beb27ca8265448af95b35b31ef718f60156`，**包含未提交修改**，不能仅用该 commit 重建本次版本。
- 基准源文件：[password_test.go.tmpl](../internal/generate/scaffold/auth/session/internal/platform/auth/password_test.go.tmpl) 中的 `BenchmarkPassword`；被测实现：[password.go.tmpl](../internal/generate/scaffold/auth/session/internal/platform/auth/password.go.tmpl)。
- 本次生成后的源码 SHA-256：`internal/platform/auth/password.go` = `3a2d410d58a27bd974777ca16e15d5fa7ce1c1e2913d7cba64b4f057bb4a9a71`；`password_test.go` = `7d483ee1dc436d33e7742b7918c339dfc6f7aaf57e2a3aafefe5dd64cbf74964`。
- 仅使用临时生成项目与虚构口令，不读真实环境文件，不连接数据库。历史结果由本记录保存，临时目录不是长期依赖。

Argon2id 使用 `m=19456 KiB,t=2,p=1`、16 字节盐、32 字节摘要。Hash 包含随机盐和编码；Verify 包含存储格式解析和比较。PBKDF2-SHA256 仅测旧格式验证，600,000 次迭代、相同盐/摘要长度；不包括验证成功后重哈希为 Argon2id 的开销。准备哈希在计时之外，串行使用 `b.Loop()`，并行使用默认 `b.RunParallel()`（worker 数为 GOMAXPROCS）。

## 可复现命令

在本仓库根目录生成隔离项目（无需 Docker 或迁移）：

```sh
task_bench_dir=$(mktemp -d /tmp/go-backend-kit-password-bench.XXXXXX)
GOBACKEND_DEVELOPMENT_REPLACE="$PWD" go run ./cmd/gobackend new "$task_bench_dir/api" \
  --module example.com/password-benchmark --auth session
cd "$task_bench_dir/api"
go tool gobackend check
go test -c -o "$task_bench_dir/auth.test" ./internal/platform/auth
GOGC=100 GOMEMLIMIT=2GiB /usr/bin/time -l "$task_bench_dir/auth.test" \
  -test.run='^$' -test.bench='^BenchmarkPassword$' -test.benchmem \
  -test.benchtime=1s -test.count=10 -test.cpu=1,4
```

本次目录是 `/tmp/go-backend-kit-password-bench.3uEQc2`；标准输出和 `time` 的标准错误合并后保存在该目录的 `raw.txt`，完整内容也附在下方。先编译再计时，所以 RSS 不包含 Go 编译器。

`/usr/bin/time -l` 为 macOS 命令。只复现跨平台 Go 基准时使用：

```sh
GOGC=100 GOMEMLIMIT=2GiB go test ./internal/platform/auth -run='^$' \
  -bench='^BenchmarkPassword$' -benchmem -benchtime=1s -count=10 -cpu=1,4
```

先完成所有性能测量，再另跑正确性/race 检查，不能混用其耗时：

```sh
go test -race ./internal/platform/auth -run='^TestPassword|^TestArgon2|^TestPBKDF2' \
  -bench='^BenchmarkPassword$' -benchtime=2x -cpu=1,4
```

该 race 命令本次通过（22.551s）；两次迭代仅用于执行基准路径，不用于性能结论。生成器现有 Session 矩阵还会运行一次迭代的 smoke，检查三种操作及串行/并行基准均存在且成功，不对速度设置 CI 阈值。

同轮仓库根目录的 `go test -race ./...` 和 `go vet ./...` 均通过（生成器测试 161.819s，包含新基准 smoke）；`git diff --check` 通过。它们在性能采样结束后执行，不计入下方统计；本轮没有重跑 Docker/浏览器或远端 CI 验收。

## 十次重复样本统计

每行 n=10。耗时取中位数，范围为实际最小/最大值，CV 为样本标准差（分母 n−1）除以均值；B/op、allocs/op 同样取各行十个值的中位数。下方原始 ns/op 可独立复算或交给 benchstat。本表没有计算跨机器差异、置信区间或优化收益。

| 操作/模式 | GOMAXPROCS | 中位数 ms/op | min–max ms/op | CV | 中位数 B/op | 中位数 allocs/op |
|---|---:|---:|---:|---:|---:|---:|
| Argon2idHash/serial | 1 | 22.289 | 22.179–22.453 | 0.44% | 19925420 | 31 |
| Argon2idHash/serial | 4 | 21.952 | 21.801–22.146 | 0.48% | 19925748 | 32 |
| Argon2idHash/parallel | 1 | 22.379 | 22.179–22.451 | 0.38% | 19925424 | 31 |
| Argon2idHash/parallel | 4 | 6.057 | 6.026–6.098 | 0.44% | 19925596 | 31 |
| Argon2idVerify/serial | 1 | 22.178 | 21.967–22.327 | 0.48% | 19925264 | 27 |
| Argon2idVerify/serial | 4 | 21.871 | 21.696–22.011 | 0.46% | 19925285 | 27 |
| Argon2idVerify/parallel | 1 | 22.198 | 22.055–22.320 | 0.37% | 19925267 | 27 |
| Argon2idVerify/parallel | 4 | 6.057 | 6.022–6.108 | 0.41% | 19925267 | 27 |
| PBKDF2Verify/serial | 1 | 62.635 | 62.370–63.295 | 0.44% | 932 | 14 |
| PBKDF2Verify/serial | 4 | 62.676 | 62.399–62.943 | 0.27% | 932 | 14 |
| PBKDF2Verify/parallel | 1 | 62.582 | 62.361–62.795 | 0.23% | 941 | 14 |
| PBKDF2Verify/parallel | 4 | 16.727 | 16.677–16.851 | 0.27% | 938.5 | 14 |

解释与限制：

- 串行默认 Argon2id 验证中位数为 **22.178 ms/op**；四 worker 并行的 **6.057 ms/op** 是总墙钟除以总完成次数，**不是单次验证的延迟**，更不是 HTTP 登录 P95/P99。
- 默认 Argon2id 验证每次分配约 19 MiB（实测 19,925,264 B/op），并行不会把每次分配缩小成四分之一。PBKDF2 的计时/分配差异不能推出安全强弱，计时也不证明 FIPS 合规。
- 整个基准进程的 macOS `maximum resident set size` 为 **241,139,712 字节（约 229.97 MiB）**。这是含全部顺序基准、GC 历史和测试运行时的进程高水位，不是某个子基准的单独峰值，也不是服务内存上限。
- 默认四个 KDF 槽位同时处理默认哈希时，Argon2 主工作缓冲区约为 19 MiB × 4 = 76 MiB。存量哈希可携带更大的受限参数，按允许的 64 MiB 上限则为 256 MiB。GC 尚未回收的缓冲区、运行时和其他业务内存都在此之外；`GOMEMLIMIT` 是软目标。
- 本基准直接执行哈希器，不测 IP/邮箱限流、PasswordManager 的 429 准入、未知账号查询、数据库、审计、HTTP 或真实部署并发。容量拒绝另有 `TestPasswordManagerRejectsQueueing`；不能用这些性能数字代替准入正确性验证。
- 没有据此修改默认参数，也没有证明目标部署达到 100–300 ms 调优目标。目标主机、真实请求负载和峰值内存仍应单独验收。

## 原始输出

```text
goos: darwin
goarch: arm64
pkg: example.com/password-benchmark/internal/platform/auth
cpu: Apple M2 Ultra
BenchmarkPassword/Argon2idHash/serial           	      50	  22388563 ns/op	19925728 B/op	      32 allocs/op
BenchmarkPassword/Argon2idHash/serial           	      50	  22330735 ns/op	19925420 B/op	      31 allocs/op
BenchmarkPassword/Argon2idHash/serial           	      50	  22273996 ns/op	19925420 B/op	      31 allocs/op
BenchmarkPassword/Argon2idHash/serial           	      54	  22179330 ns/op	19925419 B/op	      31 allocs/op
BenchmarkPassword/Argon2idHash/serial           	      52	  22249554 ns/op	19925420 B/op	      31 allocs/op
BenchmarkPassword/Argon2idHash/serial           	      50	  22451967 ns/op	19925420 B/op	      31 allocs/op
BenchmarkPassword/Argon2idHash/serial           	      50	  22452829 ns/op	19925420 B/op	      31 allocs/op
BenchmarkPassword/Argon2idHash/serial           	      52	  22259253 ns/op	19925420 B/op	      31 allocs/op
BenchmarkPassword/Argon2idHash/serial           	      52	  22303548 ns/op	19925420 B/op	      31 allocs/op
BenchmarkPassword/Argon2idHash/serial           	      54	  22182564 ns/op	19925419 B/op	      31 allocs/op
BenchmarkPassword/Argon2idHash/serial-4         	      54	  21801400 ns/op	19925752 B/op	      32 allocs/op
BenchmarkPassword/Argon2idHash/serial-4         	      50	  22146484 ns/op	19925723 B/op	      32 allocs/op
BenchmarkPassword/Argon2idHash/serial-4         	      55	  21935936 ns/op	19925749 B/op	      32 allocs/op
BenchmarkPassword/Argon2idHash/serial-4         	      52	  21941940 ns/op	19925708 B/op	      32 allocs/op
BenchmarkPassword/Argon2idHash/serial-4         	      54	  21888455 ns/op	19925725 B/op	      32 allocs/op
BenchmarkPassword/Argon2idHash/serial-4         	      52	  22029917 ns/op	19925752 B/op	      32 allocs/op
BenchmarkPassword/Argon2idHash/serial-4         	      55	  21962251 ns/op	19925747 B/op	      32 allocs/op
BenchmarkPassword/Argon2idHash/serial-4         	      54	  22089747 ns/op	19925642 B/op	      31 allocs/op
BenchmarkPassword/Argon2idHash/serial-4         	      54	  22082137 ns/op	19925767 B/op	      32 allocs/op
BenchmarkPassword/Argon2idHash/serial-4         	      55	  21939539 ns/op	19925749 B/op	      32 allocs/op
BenchmarkPassword/Argon2idHash/parallel         	      51	  22378304 ns/op	19925429 B/op	      31 allocs/op
BenchmarkPassword/Argon2idHash/parallel         	      50	  22358623 ns/op	19925423 B/op	      31 allocs/op
BenchmarkPassword/Argon2idHash/parallel         	      51	  22450796 ns/op	19925424 B/op	      31 allocs/op
BenchmarkPassword/Argon2idHash/parallel         	      52	  22282607 ns/op	19925423 B/op	      31 allocs/op
BenchmarkPassword/Argon2idHash/parallel         	      49	  22421946 ns/op	19925425 B/op	      31 allocs/op
BenchmarkPassword/Argon2idHash/parallel         	      51	  22406132 ns/op	19925424 B/op	      31 allocs/op
BenchmarkPassword/Argon2idHash/parallel         	      51	  22403661 ns/op	19925424 B/op	      31 allocs/op
BenchmarkPassword/Argon2idHash/parallel         	      51	  22379127 ns/op	19925424 B/op	      31 allocs/op
BenchmarkPassword/Argon2idHash/parallel         	      51	  22178681 ns/op	19925424 B/op	      31 allocs/op
BenchmarkPassword/Argon2idHash/parallel         	      52	  22255585 ns/op	19925423 B/op	      31 allocs/op
BenchmarkPassword/Argon2idHash/parallel-4       	     192	   6095344 ns/op	19925631 B/op	      32 allocs/op
BenchmarkPassword/Argon2idHash/parallel-4       	     192	   6097697 ns/op	19925622 B/op	      32 allocs/op
BenchmarkPassword/Argon2idHash/parallel-4       	     193	   6026288 ns/op	19925615 B/op	      32 allocs/op
BenchmarkPassword/Argon2idHash/parallel-4       	     199	   6038896 ns/op	19925536 B/op	      31 allocs/op
BenchmarkPassword/Argon2idHash/parallel-4       	     198	   6055781 ns/op	19925566 B/op	      31 allocs/op
BenchmarkPassword/Argon2idHash/parallel-4       	     192	   6030706 ns/op	19925580 B/op	      31 allocs/op
BenchmarkPassword/Argon2idHash/parallel-4       	     190	   6086895 ns/op	19925595 B/op	      31 allocs/op
BenchmarkPassword/Argon2idHash/parallel-4       	     192	   6057308 ns/op	19925626 B/op	      32 allocs/op
BenchmarkPassword/Argon2idHash/parallel-4       	     196	   6080086 ns/op	19925597 B/op	      31 allocs/op
BenchmarkPassword/Argon2idHash/parallel-4       	     192	   6046013 ns/op	19925592 B/op	      31 allocs/op
BenchmarkPassword/Argon2idVerify/serial         	      52	  21967426 ns/op	19925264 B/op	      27 allocs/op
BenchmarkPassword/Argon2idVerify/serial         	      51	  22196736 ns/op	19925264 B/op	      27 allocs/op
BenchmarkPassword/Argon2idVerify/serial         	      50	  22218645 ns/op	19925264 B/op	      27 allocs/op
BenchmarkPassword/Argon2idVerify/serial         	      52	  22072062 ns/op	19925264 B/op	      27 allocs/op
BenchmarkPassword/Argon2idVerify/serial         	      51	  22145704 ns/op	19925264 B/op	      27 allocs/op
BenchmarkPassword/Argon2idVerify/serial         	      50	  22169667 ns/op	19925264 B/op	      27 allocs/op
BenchmarkPassword/Argon2idVerify/serial         	      52	  22185552 ns/op	19925264 B/op	      27 allocs/op
BenchmarkPassword/Argon2idVerify/serial         	      51	  22326669 ns/op	19925264 B/op	      27 allocs/op
BenchmarkPassword/Argon2idVerify/serial         	      51	  22111007 ns/op	19925264 B/op	      27 allocs/op
BenchmarkPassword/Argon2idVerify/serial         	      50	  22302639 ns/op	19925264 B/op	      27 allocs/op
BenchmarkPassword/Argon2idVerify/serial-4       	      56	  21887550 ns/op	19925308 B/op	      27 allocs/op
BenchmarkPassword/Argon2idVerify/serial-4       	      54	  21696360 ns/op	19925308 B/op	      27 allocs/op
BenchmarkPassword/Argon2idVerify/serial-4       	      55	  22011251 ns/op	19925287 B/op	      27 allocs/op
BenchmarkPassword/Argon2idVerify/serial-4       	      52	  21798766 ns/op	19925284 B/op	      27 allocs/op
BenchmarkPassword/Argon2idVerify/serial-4       	      55	  21877408 ns/op	19925276 B/op	      27 allocs/op
BenchmarkPassword/Argon2idVerify/serial-4       	      51	  21723699 ns/op	19925291 B/op	      27 allocs/op
BenchmarkPassword/Argon2idVerify/serial-4       	      52	  21854092 ns/op	19925264 B/op	      27 allocs/op
BenchmarkPassword/Argon2idVerify/serial-4       	      52	  21928679 ns/op	19925286 B/op	      27 allocs/op
BenchmarkPassword/Argon2idVerify/serial-4       	      51	  21864141 ns/op	19925264 B/op	      27 allocs/op
BenchmarkPassword/Argon2idVerify/serial-4       	      52	  21966979 ns/op	19925264 B/op	      27 allocs/op
BenchmarkPassword/Argon2idVerify/parallel       	      55	  22125593 ns/op	19925266 B/op	      27 allocs/op
BenchmarkPassword/Argon2idVerify/parallel       	      51	  22226685 ns/op	19925267 B/op	      27 allocs/op
BenchmarkPassword/Argon2idVerify/parallel       	      50	  22055122 ns/op	19925267 B/op	      27 allocs/op
BenchmarkPassword/Argon2idVerify/parallel       	      51	  22156167 ns/op	19925267 B/op	      27 allocs/op
BenchmarkPassword/Argon2idVerify/parallel       	      50	  22311098 ns/op	19925267 B/op	      27 allocs/op
BenchmarkPassword/Argon2idVerify/parallel       	      52	  22319599 ns/op	19925267 B/op	      27 allocs/op
BenchmarkPassword/Argon2idVerify/parallel       	      52	  22171135 ns/op	19925267 B/op	      27 allocs/op
BenchmarkPassword/Argon2idVerify/parallel       	      50	  22201823 ns/op	19925267 B/op	      27 allocs/op
BenchmarkPassword/Argon2idVerify/parallel       	      52	  22251338 ns/op	19925267 B/op	      27 allocs/op
BenchmarkPassword/Argon2idVerify/parallel       	      52	  22193511 ns/op	19925267 B/op	      27 allocs/op
BenchmarkPassword/Argon2idVerify/parallel-4     	     189	   6079398 ns/op	19925300 B/op	      27 allocs/op
BenchmarkPassword/Argon2idVerify/parallel-4     	     195	   6052675 ns/op	19925267 B/op	      27 allocs/op
BenchmarkPassword/Argon2idVerify/parallel-4     	     193	   6059536 ns/op	19925273 B/op	      27 allocs/op
BenchmarkPassword/Argon2idVerify/parallel-4     	     193	   6063698 ns/op	19925269 B/op	      27 allocs/op
BenchmarkPassword/Argon2idVerify/parallel-4     	     195	   6054706 ns/op	19925266 B/op	      27 allocs/op
BenchmarkPassword/Argon2idVerify/parallel-4     	     195	   6021512 ns/op	19925267 B/op	      27 allocs/op
BenchmarkPassword/Argon2idVerify/parallel-4     	     193	   6036785 ns/op	19925266 B/op	      27 allocs/op
BenchmarkPassword/Argon2idVerify/parallel-4     	     192	   6107699 ns/op	19925266 B/op	      27 allocs/op
BenchmarkPassword/Argon2idVerify/parallel-4     	     190	   6035895 ns/op	19925266 B/op	      27 allocs/op
BenchmarkPassword/Argon2idVerify/parallel-4     	     193	   6072531 ns/op	19925267 B/op	      27 allocs/op
BenchmarkPassword/PBKDF2Verify/serial           	      19	  62434906 ns/op	     932 B/op	      14 allocs/op
BenchmarkPassword/PBKDF2Verify/serial           	      18	  63294611 ns/op	     932 B/op	      14 allocs/op
BenchmarkPassword/PBKDF2Verify/serial           	      19	  62666583 ns/op	     932 B/op	      14 allocs/op
BenchmarkPassword/PBKDF2Verify/serial           	      18	  62639192 ns/op	     932 B/op	      14 allocs/op
BenchmarkPassword/PBKDF2Verify/serial           	      18	  62369662 ns/op	     932 B/op	      14 allocs/op
BenchmarkPassword/PBKDF2Verify/serial           	      19	  62945241 ns/op	     932 B/op	      14 allocs/op
BenchmarkPassword/PBKDF2Verify/serial           	      18	  62572602 ns/op	     932 B/op	      14 allocs/op
BenchmarkPassword/PBKDF2Verify/serial           	      18	  62630968 ns/op	     935 B/op	      14 allocs/op
BenchmarkPassword/PBKDF2Verify/serial           	      18	  62418623 ns/op	     932 B/op	      14 allocs/op
BenchmarkPassword/PBKDF2Verify/serial           	      18	  62715227 ns/op	     932 B/op	      14 allocs/op
BenchmarkPassword/PBKDF2Verify/serial-4         	      18	  62533597 ns/op	     933 B/op	      14 allocs/op
BenchmarkPassword/PBKDF2Verify/serial-4         	      18	  62560931 ns/op	     932 B/op	      14 allocs/op
BenchmarkPassword/PBKDF2Verify/serial-4         	      19	  62813011 ns/op	     932 B/op	      14 allocs/op
BenchmarkPassword/PBKDF2Verify/serial-4         	      18	  62586907 ns/op	     932 B/op	      14 allocs/op
BenchmarkPassword/PBKDF2Verify/serial-4         	      18	  62943046 ns/op	     932 B/op	      14 allocs/op
BenchmarkPassword/PBKDF2Verify/serial-4         	      18	  62399157 ns/op	     935 B/op	      14 allocs/op
BenchmarkPassword/PBKDF2Verify/serial-4         	      18	  62649745 ns/op	     932 B/op	      14 allocs/op
BenchmarkPassword/PBKDF2Verify/serial-4         	      18	  62702130 ns/op	     932 B/op	      14 allocs/op
BenchmarkPassword/PBKDF2Verify/serial-4         	      18	  62825405 ns/op	     932 B/op	      14 allocs/op
BenchmarkPassword/PBKDF2Verify/serial-4         	      18	  62815468 ns/op	     932 B/op	      14 allocs/op
BenchmarkPassword/PBKDF2Verify/parallel         	      18	  62516370 ns/op	     941 B/op	      14 allocs/op
BenchmarkPassword/PBKDF2Verify/parallel         	      18	  62620801 ns/op	     941 B/op	      14 allocs/op
BenchmarkPassword/PBKDF2Verify/parallel         	      18	  62626352 ns/op	     941 B/op	      14 allocs/op
BenchmarkPassword/PBKDF2Verify/parallel         	      18	  62409426 ns/op	     941 B/op	      14 allocs/op
BenchmarkPassword/PBKDF2Verify/parallel         	      19	  62360855 ns/op	     940 B/op	      14 allocs/op
BenchmarkPassword/PBKDF2Verify/parallel         	      18	  62543308 ns/op	     941 B/op	      14 allocs/op
BenchmarkPassword/PBKDF2Verify/parallel         	      19	  62695086 ns/op	     940 B/op	      14 allocs/op
BenchmarkPassword/PBKDF2Verify/parallel         	      18	  62745979 ns/op	     941 B/op	      14 allocs/op
BenchmarkPassword/PBKDF2Verify/parallel         	      18	  62795375 ns/op	     941 B/op	      14 allocs/op
BenchmarkPassword/PBKDF2Verify/parallel         	      19	  62438778 ns/op	     940 B/op	      14 allocs/op
BenchmarkPassword/PBKDF2Verify/parallel-4       	      66	  16732621 ns/op	     938 B/op	      14 allocs/op
BenchmarkPassword/PBKDF2Verify/parallel-4       	      66	  16722672 ns/op	     939 B/op	      14 allocs/op
BenchmarkPassword/PBKDF2Verify/parallel-4       	      66	  16705440 ns/op	     953 B/op	      14 allocs/op
BenchmarkPassword/PBKDF2Verify/parallel-4       	      69	  16850646 ns/op	     960 B/op	      14 allocs/op
BenchmarkPassword/PBKDF2Verify/parallel-4       	      66	  16710178 ns/op	     961 B/op	      14 allocs/op
BenchmarkPassword/PBKDF2Verify/parallel-4       	      66	  16750101 ns/op	     938 B/op	      14 allocs/op
BenchmarkPassword/PBKDF2Verify/parallel-4       	      66	  16676696 ns/op	     940 B/op	      14 allocs/op
BenchmarkPassword/PBKDF2Verify/parallel-4       	      66	  16728640 ns/op	     938 B/op	      14 allocs/op
BenchmarkPassword/PBKDF2Verify/parallel-4       	      66	  16725103 ns/op	     938 B/op	      14 allocs/op
BenchmarkPassword/PBKDF2Verify/parallel-4       	      66	  16743499 ns/op	     938 B/op	      14 allocs/op
PASS
      149.95 real       276.87 user         1.84 sys
           241139712  maximum resident set size
                   0  average shared memory size
                   0  average unshared data size
                   0  average unshared stack size
               15027  page reclaims
                   0  page faults
                   0  swaps
                   0  block input operations
                   0  block output operations
                   0  messages sent
                   0  messages received
               33030  signals received
               20198  voluntary context switches
              172028  involuntary context switches
       3674450890128  instructions retired
        933201750828  cycles elapsed
           231277264  peak memory footprint
```
