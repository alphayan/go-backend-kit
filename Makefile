# 仓库级开发质量工具链（仅 go-backend-kit 自身，不含生成器产物）。
# 上游：https://github.com/alphayan/make-toolkit
# vendored commit：a535269
#
# 生成项目 Makefile（internal/generate/scaffold/Makefile.tmpl）不在本次范围：
# 那里已有 test/vet/vuln 目标，直接 include 会重名并改变语义。

# >>> make-toolkit >>>
# 显式覆盖默认排除，确保 cmd/internal 等全部 Go 包都被测试/检查。
# a^ 是永不匹配的 ERE，避免默认 COVERAGE_EXCLUDE 含 /cmd 导致假绿。
GO_MODULES := .
FORMAT_MODULES := .
TEST_MODULES := .
COVERAGE_EXCLUDE := a^
QUALITY_EXCLUDE := a^
RACE_EXCLUDE := a^
GOLANGCI_LINT_VERSION := v2.12.2

include tools/make-toolkit/quality.mk
# <<< make-toolkit <<<
