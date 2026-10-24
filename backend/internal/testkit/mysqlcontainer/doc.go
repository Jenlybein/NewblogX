// Package mysqlcontainer 提供集成测试用的 MySQL 8 容器。
//
// 具体实现只在 `-tags integration` 下编译（见 mysqlcontainer.go）；
// 本文件保证包在任何构建模式下都存在，避免 `go build ./...` 报
// "build constraints exclude all Go files"。
package mysqlcontainer
