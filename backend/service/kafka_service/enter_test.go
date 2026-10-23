package kafka_service_test

import (
	"context"
	"myblogx/service/kafka_service"
	"testing"
	"time"

	"github.com/segmentio/kafka-go"
)

func TestKafkaClientSendNilWriter(t *testing.T) {
	var client *kafka_service.KafkaMysqlClient
	if err := client.Send(context.Background(), "k", []byte("v")); err == nil {
		t.Fatal("nil client 应返回错误")
	}

	client = &kafka_service.KafkaMysqlClient{}
	if err := client.Send(context.Background(), "k", []byte("v")); err == nil {
		t.Fatal("nil writer 应返回错误")
	}
}

func TestKafkaClientCloseNil(t *testing.T) {
	var client *kafka_service.KafkaMysqlClient
	if err := client.Close(); err != nil {
		t.Fatalf("nil client Close 不应报错: %v", err)
	}
}

func TestKafkaClientSendWriterErrorAndClose(t *testing.T) {
	client := &kafka_service.KafkaMysqlClient{
		Writer: &kafka.Writer{
			Addr:     kafka.TCP("127.0.0.1:1"),
			Topic:    "unit-topic",
			Balancer: &kafka.LeastBytes{},
		},
		Reader: kafka.NewReader(kafka.ReaderConfig{
			Brokers: []string{"127.0.0.1:1"},
			Topic:   "unit-topic",
			GroupID: "g1",
		}),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if err := client.Send(ctx, "k", []byte("v")); err == nil {
		t.Fatal("不可达 broker 应返回发送错误")
	}

	if err := client.Close(); err != nil {
		t.Fatalf("Close 不应报错: %v", err)
	}
}
