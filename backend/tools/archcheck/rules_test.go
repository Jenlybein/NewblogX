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

// TestAllRulesHaveSelfCheck 确保每条规则都有测试覆盖。
//
// 这是防止"以后新增规则却忘了写测试"的元规则。
func TestAllRulesHaveSelfCheck(t *testing.T) {
	covered := map[string]bool{
		"R1": true, "R2": true, "R3": true, "R4": true,
		"R5": true, "R6": true, "R7": true, "R8": true,
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
