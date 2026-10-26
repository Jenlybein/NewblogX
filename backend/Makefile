# BlogX 后端常用命令
#
# 说明：openapi 契约由 swag 从代码注解生成，产物落在 docs/swagger.json。
# 两个前端（frontend / frontend_admin）的类型生成直接消费它。

# ⚠️ 必须用 swag **v2**：--v3.1 让 swag 直出 OpenAPI 3.1，
# 前端因此可以省掉 swagger2openapi 这一步。
#
# v1 只输出 Swagger 2.0，而 openapi-typescript 只吃 3.x —— 装成 v1 时
# `make openapi` 会退化成 2.0 产物，前端 `pnpm openapi:types` 会报错。
#
# 安装：go install github.com/swaggo/swag/v2/cmd/swag@v2.0.0-rc6
SWAG ?= $(shell go env GOPATH)/bin/swag

# 只出 json：docs.go 无人 import（gin 里没有挂 /swagger 路由），
# swagger.yaml 也没有消费方。少了这两个产物，go.mod 里那个 swag 依赖也一起没了。
SWAG_OUTPUT_TYPES ?= json

.PHONY: openapi build test gate

# 重新生成 OpenAPI 契约
openapi:
	$(SWAG) init -g cmd/server/main.go -o docs --parseDependency --parseInternal \
		--v3.1 --outputTypes $(SWAG_OUTPUT_TYPES)

build:
	go build ./...

test:
	go test ./... -count=1

# 本地全量门禁（含 archcheck 与前端）走仓库根脚本
gate:
	bash ../scripts/check.sh

