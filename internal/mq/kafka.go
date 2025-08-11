package mq

import (
	"strings"

	"github.com/IBM/sarama"
)

// KafkaProducer 简易封装
type KafkaProducer struct {
	Async sarama.AsyncProducer
	Topic string
}

func NewKafkaProducer(brokersCSV, topic string) (*KafkaProducer, error) {
	brokers := []string{}
	if brokersCSV != "" {
		brokers = strings.Split(brokersCSV, ",")
	}
	cfg := sarama.NewConfig()
	cfg.Producer.Return.Successes = false
	cfg.Producer.Return.Errors = false
	p, err := sarama.NewAsyncProducer(brokers, cfg)
	if err != nil {
		return nil, err
	}
	return &KafkaProducer{Async: p, Topic: topic}, nil
}

func (p *KafkaProducer) Publish(value []byte, key []byte) {
	if p == nil || p.Async == nil {
		return
	}
	p.Async.Input() <- &sarama.ProducerMessage{Topic: p.Topic, Key: sarama.ByteEncoder(key), Value: sarama.ByteEncoder(value)}
}

func (p *KafkaProducer) Close() error {
	if p == nil || p.Async == nil {
		return nil
	}
	return p.Async.Close()
}
