package service_test

import (
	"context"
	"myblogx/service/kafka_service"
	"testing"
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
