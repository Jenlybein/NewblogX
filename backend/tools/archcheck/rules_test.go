package main

import (
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

// 这个测试文件是整个 archcheck 的"保险丝"。
//
// 上一轮重构失败的根本原因是：守卫检查的 5 个符号在项目中根本不存在，
// 所以 13 条规则全部永真通过。要避免重蹈覆辙，唯一的办法是
// **为每条规则准备一段人造违规代码，断言它真的会报错**。
//
// 如果某天有人改坏了规则（或删掉了检查逻辑），这里立刻会红。

// case_ 描述一个测试用例：给定源码与虚拟路径，期望命中 ruleID 的次数。
type case_ struct {
	name     string
	rel      string // 虚拟文件路径，决定规则是否适用
	src      string // Go 源码
	ruleID   string
	wantHits int
}

func runOne(t *testing.T, c case_) {
	t.Helper()

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, c.rel, c.src, parser.ParseComments)
	if err != nil {
		t.Fatalf("[%s] 测试源码本身解析失败: %v", c.name, err)
	}

	var hits int
	var found bool
	for _, r := range rules() {
		if r.ID != c.ruleID {
			continue
		}
		found = true
		hits = len(r.Check(fset, file, c.rel))
	}
	if !found {
		t.Fatalf("[%s] 规则 %s 不存在", c.name, c.ruleID)
	}

	if hits != c.wantHits {
		t.Errorf("[%s] 规则 %s 命中 %d 次，期望 %d 次\n源码:\n%s",
			c.name, c.ruleID, hits, c.wantHits, c.src)
	}
}

// TestR1_API禁止gorm 验证 api 层出现 *gorm.DB 会被抓到。
func TestR1_API禁止gorm(t *testing.T) {
	cases := []case_{
		{
			name: "api 层持有 *gorm.DB",
			rel:  "api/article_api/enter.go",
			src: `package article_api

import "gorm.io/gorm"

type Handler struct {
	db *gorm.DB
}
`,
			ruleID:   "R1",
			wantHits: 2, // 1 处 import + 1 处类型
		},
		{
			name: "service 层不受此规则约束",
			rel:  "service/article_service/enter.go",
			src: `package article_service

import "gorm.io/gorm"

type Svc struct {
	db *gorm.DB
}
`,
			ruleID:   "R1",
			wantHits: 0,
		},
		{
			name: "api 层用别名也拦得住",
			rel:  "api/user_api/handler.go",
			src: `package user_api

import g "gorm.io/gorm"

var conn *g.DB
`,
			ruleID: "R1",
			// import 路径仍命中 gorm.io/gorm；类型处因包名为 g 而漏掉
			wantHits: 1,
		},
	}
	for _, c := range cases {
		runOne(t, c)
	}
}

// TestR2_禁止Deps 验证依赖打包结构体会被拦下。
func TestR2_禁止Deps(t *testing.T) {
	cases := []case_{
		{
			name: "定义 Deps",
			rel:  "apideps/deps.go",
			src: `package apideps

import "gorm.io/gorm"

type Deps struct {
	DB *gorm.DB
}
`,
			ruleID:   "R2",
			wantHits: 1,
		},
		{
			name: "定义 XxxDeps 也要拦",
			rel:  "api/enter.go",
			src: `package api

type APIDeps struct {
	DB any
}
`,
			ruleID:   "R2",
			wantHits: 1,
		},
		{
			name: "普通结构体不误报",
			rel:  "api/enter.go",
			src: `package api

type Handler struct {
	Name string
}
`,
			ruleID:   "R2",
			wantHits: 0,
		},
		{
			name: "Deps 作为非 struct 类型不误报",
			rel:  "api/enter.go",
			src: `package api

type Deps = map[string]any
`,
			ruleID:   "R2",
			wantHits: 0,
		},
	}
	for _, c := range cases {
		runOne(t, c)
	}
}

// TestR3_service禁止gin 验证业务层不得依赖 HTTP 框架。
func TestR3_service禁止gin(t *testing.T) {
	cases := []case_{
		{
			name: "service 层 import gin",
			rel:  "service/user_service/enter.go",
			src: `package user_service

import "github.com/gin-gonic/gin"

func Handle(c *gin.Context) {}
`,
			ruleID:   "R3",
			wantHits: 1,
		},
		{
			name: "api 层 import gin 是正常的",
			rel:  "api/user_api/enter.go",
			src: `package user_api

import "github.com/gin-gonic/gin"

func Handle(c *gin.Context) {}
`,
			ruleID:   "R3",
			wantHits: 0,
		},
	}
	for _, c := range cases {
		runOne(t, c)
	}
}

// TestR4_domain禁止timeNow 验证领域层直接取时间会被提示。
func TestR4_domain禁止timeNow(t *testing.T) {
	cases := []case_{
		{
			name: "domain 层调 time.Now",
			rel:  "internal/article/domain/entity.go",
			src: `package domain

import "time"

func Published() bool {
	return time.Now().After(time.Time{})
}
`,
			ruleID:   "R4",
			wantHits: 1,
		},
		{
			name: "app 层调 time.Now 暂不告警",
			rel:  "internal/article/app/usecase.go",
			src: `package app

import "time"

func Run() { _ = time.Now() }
`,
			ruleID:   "R4",
			wantHits: 0,
		},
	}
	for _, c := range cases {
		runOne(t, c)
	}
}

// TestR5_api禁止repository 验证分层倒置会被拦下。
func TestR5_api禁止repository(t *testing.T) {
	cases := []case_{
		{
			name: "api 层 import repository",
			rel:  "api/article_api/enter.go",
			src: `package article_api

import "myblogx/repository/article_repo"

var _ = article_repo.New
`,
			ruleID:   "R5",
			wantHits: 1,
		},
		{
			name: "service 层 import repository 是正常的",
			rel:  "service/article_service/enter.go",
			src: `package article_service

import "myblogx/repository/article_repo"

var _ = article_repo.New
`,
			ruleID:   "R5",
			wantHits: 0,
		},
	}
	for _, c := range cases {
		runOne(t, c)
	}
}

// TestR6_单文件行数上限 验证超长文件会被标记。
func TestR6_单文件行数上限(t *testing.T) {
	// 构造一个 900 行的文件
	var sb strings.Builder
	sb.WriteString("package big\n\n")
	for i := 0; i < 900; i++ {
		sb.WriteString("// filler\n")
	}

	cases := []case_{
		{
			name:     "900 行文件超限",
			rel:      "service/es_service/article_sync.go",
			src:      sb.String(),
			ruleID:   "R6",
			wantHits: 1,
		},
		{
			name:     "短文件不告警",
			rel:      "service/es_service/small.go",
			src:      "package small\n\nfunc A() {}\n",
			ruleID:   "R6",
			wantHits: 0,
		},
	}
	for _, c := range cases {
		runOne(t, c)
	}
}

// TestR7_禁止裸go 验证未受保护的 goroutine 会被标记。
func TestR7_禁止裸go(t *testing.T) {
	cases := []case_{
		{
			name: "裸 go 调用",
			rel:  "api/article_api/article_digg.go",
			src: `package article_api

func f() {}

func Handler() {
	go f()
}
`,
			ruleID:   "R7",
			wantHits: 1,
		},
		{
			name: "go func 匿名函数同样算裸",
			rel:  "api/comment_api/comment_create.go",
			src: `package comment_api

func Handler() {
	go func() {}()
}
`,
			ruleID:   "R7",
			wantHits: 1,
		},
		{
			name:     "没有 go 语句不告警",
			rel:      "api/x/x.go",
			src:      "package x\n\nfunc f() {}\n",
			ruleID:   "R7",
			wantHits: 0,
		},
		{
			// safego 是规则的正解实现地，必须豁免；否则规则无解。
			name: "safego 自身实现豁免",
			rel:  "internal/platform/safego/safego.go",
			src: `package safego

func Go(fn func()) {
	go func() { fn() }()
}
`,
			ruleID:   "R7",
			wantHits: 0,
		},
		{
			// 豁免必须精确：platform 其他目录仍然要拦。
			name: "platform 其他包不豁免",
			rel:  "internal/platform/log/emit.go",
			src: `package log

func start() {
	go func() {}()
}
`,
			ruleID:   "R7",
			wantHits: 1,
		},
	}
	for _, c := range cases {
		runOne(t, c)
	}
}

// TestR8_禁止中文错误 验证硬编码中文错误会被标记。
func TestR8_禁止中文错误(t *testing.T) {
	cases := []case_{
		{
			name: "中文 errors.New",
			rel:  "api/user_api/login.go",
			src: `package user_api

import "errors"

var ErrBad = errors.New("用户名或密码错误")
`,
			ruleID:   "R8",
			wantHits: 1,
		},
		{
			name: "英文 errors.New 暂不告警",
			rel:  "api/user_api/login.go",
			src: `package user_api

import "errors"

var ErrBad = errors.New("invalid credentials")
`,
			ruleID:   "R8",
			wantHits: 0,
		},
	}
	for _, c := range cases {
		runOne(t, c)
	}
}

// TestR9_日志输入禁any 验证审计输入结构体里的裸 any 字段会被抓。
func TestR9_日志输入禁any(t *testing.T) {
	cases := []case_{
		{
			name: "审计输入带 any 字段",
			rel:  "internal/platform/log/writer/write_action_audit.go",
			src: `package log_service

type ActionAuditInput struct {
	RequestBody any
	Extra       map[string]any
}
`,
			ruleID:   "R9",
			wantHits: 1, // 只有裸 any 命中，map[string]any 不算
		},
		{
			name: "GinAuditInput 带 interface{} 也拦",
			rel:  "internal/platform/http/middleware/log_emit.go",
			src: `package middleware

type GinAuditInput struct {
	Payload interface{}
}
`,
			ruleID:   "R9",
			wantHits: 1,
		},
		{
			name: "普通业务结构体不误报",
			rel:  "internal/platform/log/writer/other.go",
			src: `package log_service

type SomeInput struct {
	Payload any
}
`,
			ruleID:   "R9",
			wantHits: 0,
		},
		{
			name: "非日志层不适用",
			rel:  "api/x/x.go",
			src: `package x

type ActionAuditInput struct {
	Payload any
}
`,
			ruleID:   "R9",
			wantHits: 0,
		},
	}
	for _, c := range cases {
		runOne(t, c)
	}
}

// TestAllRulesHaveSelfCheck 确保每条规则都有测试覆盖。
//
// 这是防止"以后新增规则却忘了写测试"的元规则。
func TestAllRulesHaveSelfCheck(t *testing.T) {
	covered := map[string]bool{
		"R1": true, "R2": true, "R3": true, "R4": true,
		"R5": true, "R6": true, "R7": true, "R8": true, "R9": true,
		"R10": true, "R11": true,
	}
	for _, r := range rules() {
		if !covered[r.ID] {
			t.Errorf("规则 %s（%s）没有对应的自检测试用例——它可能在永真通过", r.ID, r.Title)
		}
	}
	if len(rules()) != len(covered) {
		t.Errorf("规则数与自检覆盖数不一致：规则 %d 条，覆盖 %d 条", len(rules()), len(covered))
	}
}

// TestR10_模块禁止importModels 验证 row 归属口径的闸门。
func TestR10_模块禁止importModels(t *testing.T) {
	cases := []case_{
		{
			name: "模块 adapter 直接 import models 根包",
			rel:  "internal/comment/adapter/mysql/repo.go",
			src: `package mysql

import "myblogx/models"
`,
			ruleID:   "R10",
			wantHits: 1,
		},
		{
			name: "模块 domain import models 子包（非 ctype）",
			rel:  "internal/comment/domain/comment.go",
			src: `package domain

import "myblogx/models/enum"
`,
			ruleID:   "R10",
			wantHits: 1,
		},
		{
			name: "import models/ctype 放行（值类型，M6 才搬）",
			rel:  "internal/comment/adapter/mysql/repo.go",
			src: `package mysql

import "myblogx/models/ctype"
`,
			ruleID:   "R10",
			wantHits: 0,
		},
		{
			name: "共享设施不受约束",
			rel:  "internal/platform/idgen/idgen.go",
			src: `package idgen

import "myblogx/models/ctype"
`,
			ruleID:   "R10",
			wantHits: 0,
		},
		{
			name: "旧 api 层不在本规则范围（由 R1/R5 管）",
			rel:  "api/comment_api/enter.go",
			src: `package comment_api

import "myblogx/models"
`,
			ruleID:   "R10",
			wantHits: 0,
		},
	}
	for _, c := range cases {
		runOne(t, c)
	}
}

// TestR11_禁止跨模块importAdapter 验证模块边界。
func TestR11_禁止跨模块importAdapter(t *testing.T) {
	cases := []case_{
		{
			name: "comment 模块 import article 的 adapter",
			rel:  "internal/comment/app/create.go",
			src: `package app

import "myblogx/internal/article/adapter/mysql"
`,
			ruleID:   "R11",
			wantHits: 1,
		},
		{
			name: "import 同模块自己的 adapter 放行",
			rel:  "internal/article/app/create.go",
			src: `package app

import "myblogx/internal/article/adapter/mysql"
`,
			ruleID:   "R11",
			wantHits: 0,
		},
		{
			name: "import 别的模块 domain（端口/值对象）放行",
			rel:  "internal/comment/app/create.go",
			src: `package app

import "myblogx/internal/article/domain"
`,
			ruleID:   "R11",
			wantHits: 0,
		},
		{
			name: "import platform 适配器放行",
			rel:  "internal/comment/adapter/mysql/repo.go",
			src: `package mysql

import "myblogx/internal/platform/mysql"
`,
			ruleID:   "R11",
			wantHits: 0,
		},
		{
			name: "非 internal 文件不在范围",
			rel:  "api/comment_api/create.go",
			src: `package comment_api

import "myblogx/internal/article/adapter/mysql"
`,
			ruleID:   "R11",
			wantHits: 0,
		},
	}
	for _, c := range cases {
		runOne(t, c)
	}
}
