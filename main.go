package main

// Inspired by https://github.com/wvanbergen/kafka/blob/master/examples/consumergroup/main.go

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/Shopify/sarama"
	"github.com/wvanbergen/kafka/consumergroup"
	"github.com/wvanbergen/kazoo-go"
)

const (
	DefaultKafkaTopics   = "test_topic"
	DefaultConsumerGroup = "consumer_example.go"
)

var (
	consumerGroup  = flag.String("group", DefaultConsumerGroup, "The name of the consumer group, used for coordination and load balancing")
	kafkaTopicsCSV = flag.String("topics", DefaultKafkaTopics, "The comma-separated list of topics to consume")
	zookeeper      = flag.String("zookeeper", "", "A comma-separated Zookeeper connection string (e.g. `zookeeper1.local:2181,zookeeper2.local:2181,zookeeper3.local:2181`)")

	zookeeperNodes []string
)

func init() {
	sarama.Logger = log.New(os.Stdout, "[Sarama] ", log.LstdFlags)
}

func main() {
	flag.Parse()

	if *zookeeper == "" {
		flag.PrintDefaults()
		os.Exit(1)
	}

	config := consumergroup.NewConfig()
	config.Offsets.Initial = sarama.OffsetNewest
	config.Offsets.ProcessingTimeout = 10 * time.Second

	zookeeperNodes, config.Zookeeper.Chroot = kazoo.ParseConnectionString(*zookeeper)

	kafkaTopics := strings.Split(*kafkaTopicsCSV, ",")

	consumer, consumerErr := consumergroup.JoinConsumerGroup(*consumerGroup, kafkaTopics, zookeeperNodes, config)
	if consumerErr != nil {
		log.Fatalln(consumerErr)
	}

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	doneCh := make(chan struct{})
	eventCount := 0
	offsets := make(map[string]map[int32]int64)
	//
	go func() {
		for {
			select {
			case err := <-consumer.Errors():
				log.Println("Error closing the consumer", err)
				if err := consumer.Close(); err != nil {
					sarama.Logger.Println("Error closing the consumer", err)
				}
			case message := <-consumer.Messages():
				if offsets[message.Topic] == nil {
					offsets[message.Topic] = make(map[int32]int64)
				}
				eventCount++
				if offsets[message.Topic][message.Partition] != 0 && offsets[message.Topic][message.Partition] != message.Offset-1 {
					log.Printf("Unexpected offset on %s:%d. Expected %d, found %d, diff %d.\n", message.Topic, message.Partition, offsets[message.Topic][message.Partition]+1, message.Offset, message.Offset-offsets[message.Topic][message.Partition]+1)
				}
				sarama.Logger.Println("Received message", string(message.Key), string(message.Value))

				// Simulate processing time
				time.Sleep(10 * time.Millisecond)
		
				offsets[message.Topic][message.Partition] = message.Offset
				consumer.CommitUpto(message)
				log.Printf("Offsets: %+v", offsets)
			case <-c:
				sarama.Logger.Println("Interrupt is detected")
				doneCh <- struct{}{}
			}
		}
	}()
	<-doneCh
	sarama.Logger.Println("Processed", eventCount, "messages")
}
