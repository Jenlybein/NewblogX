package core

import (
	"context"
	"time"

	"myblogx/conf"
	"myblogx/service/kafka_service"

	"github.com/segmentio/kafka-go"
	"github.com/segmentio/kafka-go/sasl/plain"
	"github.com/sirupsen/logrus"
)

func KafkaMysqlClientInit(kafkaCfg *conf.Kafka, logger *logrus.Logger) *kafka_service.KafkaMysqlClient {
	// 配置校验
	if len(kafkaCfg.Mysql.Brokers) == 0 {
		logger.Warn("Kafka Broker 列表为空，不初始化 MySQL Kafka 客户端")
		return nil
	}
	if kafkaCfg.Mysql.Topic == "" {
		logger.Warn("Kafka 主题为空，不初始化 MySQL Kafka 客户端")
		return nil
	}
	if kafkaCfg.Mysql.GroupID == "" {
		logger.Warn("Kafka 消费组 ID 为空，不初始化 MySQL Kafka 客户端")
		return nil
	}

	// 校验配置

	dialer := &kafka.Dialer{
		Timeout:   10 * time.Second,
		DualStack: true,
	}
	transport := &kafka.Transport{}

	if kafkaCfg.Mysql.Username != "" {
		mechanism := plain.Mechanism{
			Username: kafkaCfg.Mysql.Username,
			Password: kafkaCfg.Mysql.Password,
		}
		dialer.SASLMechanism = mechanism
		transport.SASL = mechanism
	}

	// 启动时校验 broker 连通性
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	conn, err := dialer.DialContext(ctx, "tcp", kafkaCfg.Mysql.Brokers[0])
	if err != nil {
		logger.Fatalf("Kafka 连接失败: %v", err)
		return nil
	}
	_ = conn.Close()

	logger.Infof("Kafka 连接成功: %s", kafkaCfg.Mysql.Brokers[0])

	writer := &kafka.Writer{
		Addr:                   kafka.TCP(kafkaCfg.Mysql.Brokers...),
		Topic:                  kafkaCfg.Mysql.Topic,
		Balancer:               &kafka.LeastBytes{},
		RequiredAcks:           kafka.RequireOne,
		MaxAttempts:            3,
		WriteBackoffMin:        500 * time.Millisecond,
		WriteBackoffMax:        500 * time.Millisecond,
		Transport:              transport,
		AllowAutoTopicCreation: false,
	}

	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:        kafkaCfg.Mysql.Brokers,
		Topic:          kafkaCfg.Mysql.Topic,
		GroupID:        kafkaCfg.Mysql.GroupID,
		MinBytes:       1,
		MaxBytes:       1024 * 1024,
		MaxWait:        200 * time.Millisecond,
		CommitInterval: 1 * time.Second,
		Dialer:         dialer,
	})

	return &kafka_service.KafkaMysqlClient{
		Writer: writer,
		Reader: reader,
	}
}
