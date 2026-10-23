package kafka_service

import (
	"context"
	"errors"
	"time"

	"github.com/segmentio/kafka-go"
)

type KafkaMysqlClient struct {
	Writer *kafka.Writer
	Reader *kafka.Reader
}

func (k *KafkaMysqlClient) Send(ctx context.Context, key string, value []byte) error {
	if k == nil || k.Writer == nil {
		return errors.New("kafka writer 未初始化")
	}

	return k.Writer.WriteMessages(ctx, kafka.Message{
		Key:   []byte(key),
		Value: value,
		Time:  time.Now(),
	})
}

func (k *KafkaMysqlClient) Close() error {
	if k == nil {
		return nil
	}

	var closeErr error
	if k.Writer != nil {
		if err := k.Writer.Close(); err != nil {
			closeErr = err
		}
	}
	if k.Reader != nil {
		if err := k.Reader.Close(); err != nil && closeErr == nil {
			closeErr = err
		}
	}

	return closeErr
}
