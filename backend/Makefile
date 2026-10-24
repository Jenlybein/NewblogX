# BlogX 后端常用命令
#
# 说明：openapi 契约由 swag 从代码注解生成，产物落在 docs/（docs.go / swagger.json / swagger.yaml）。
# 两个前端（frontend / frontend_admin）的 S2.9 类型生成直接消费 docs/swagger.json。

SWAG ?= $(shell go env GOPATH)/bin/swag

.PHONY: openapi build test gate

# 重新生成 OpenAPI 契约
openapi:
	$(SWAG) init -g cmd/server/main.go -o docs --parseDependency --parseInternal

build:
	go build ./...

test:
	go test ./... -count=1

# 本地全量门禁（含 archcheck 与前端）走仓库根脚本
gate:
	bash ../scripts/check.sh
