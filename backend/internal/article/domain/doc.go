// Package domain 是 article 模块的领域层。
//
// 铁律：
//   - 只依赖标准库。不 import gorm、gin、models、service。
//   - 不直接取时间、不直接生成 ID：时间由调用方传入，ID 由 app 传入。
//   - 业务规则写成只读的纯函数（rule.go），状态迁移写成聚合根的方法（article.go）。
//
// 这样 domain 测试可以 `t.Parallel()` 跑满 CPU，且不需要容器、DB、HTTP。
package domain
