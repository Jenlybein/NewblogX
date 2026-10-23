package conf

type Kafka struct {
	Mysql KafkaConf `yaml:"mysql"`
}

type KafkaConf struct {
	Brokers  []string `yaml:"brokers"`
	Topic    string   `yaml:"topic"`
	GroupID  string   `yaml:"group_id"`
	Username string   `yaml:"username"`
	Password string   `yaml:"password"`
}
