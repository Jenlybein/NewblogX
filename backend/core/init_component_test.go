package core_test

import (
	"fmt"
	"myblogx/conf"
	"myblogx/core"
	"myblogx/test/testutil"
	"net"
	"testing"
)

func TestInitRedis(t *testing.T) {
	mr := testutil.SetupMiniRedis(t)

	host, port, err := net.SplitHostPort(mr.Addr())
	if err != nil {
		t.Fatalf("解析 miniredis 地址失败: %v", err)
	}
	p := 0
	if _, err := fmt.Sscanf(port, "%d", &p); err != nil {
		t.Fatalf("解析端口失败: %v", err)
	}

	client := core.InitRedis(&conf.Redis{
		Addr: host,
		Port: p,
		DB:   0,
	}, testutil.Logger())
	if client == nil {
		t.Fatal("InitRedis 不应返回 nil")
	}
	if err := client.Set(t.Context(), "k", "v", 0).Err(); err != nil {
		t.Fatalf("redis 写入失败: %v", err)
	}
}

func TestKafkaMysqlClientInitValidation(t *testing.T) {
	testutil.InitGlobals()

	if got := core.KafkaMysqlClientInit(&conf.Kafka{}, testutil.Logger()); got != nil {
		t.Fatalf("brokers 为空时应返回 nil")
	}

	if got := core.KafkaMysqlClientInit(&conf.Kafka{
		Mysql: conf.KafkaConf{Brokers: []string{"127.0.0.1:9092"}},
	}, testutil.Logger()); got != nil {
		t.Fatalf("topic 为空时应返回 nil")
	}

	if got := core.KafkaMysqlClientInit(&conf.Kafka{
		Mysql: conf.KafkaConf{
			Brokers: []string{"127.0.0.1:9092"},
			Topic:   "t1",
		},
	}, testutil.Logger()); got != nil {
		t.Fatalf("group_id 为空时应返回 nil")
	}
}

func TestEsConnectNilAddresses(t *testing.T) {
	testutil.InitGlobals()
	if got := core.EsConnect(&conf.ES{}, testutil.Logger()); got != nil {
		t.Fatalf("ES 地址为空时应返回 nil")
	}
}
