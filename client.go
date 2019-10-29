package logit

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sync/atomic"
	"time"

	"github.com/intwinelabs/logger"
	"github.com/streadway/amqp"
	"gocloud.dev/pubsub"
	"gocloud.dev/pubsub/rabbitpubsub"
)

type LogitClient struct {
	log           *logger.Logger
	ampqConn      *amqp.Connection
	subscriptions []*pubsub.Subscription
	control       *pubsub.Topic
	errChan       chan error
	stop          chan bool
	startTime     time.Time
	logCnt        uint32
}

func NewClient(expr string, topics []string, log *logger.Logger) (*LogitClient, chan error, error) {
	// create rabbit mq conn
	rabbitConn, err := amqp.Dial("amqp://guest:guest@localhost:5672/")
	if err != nil {
		return nil, nil, err
	}

	// connect to the control topic
	control := rabbitpubsub.OpenTopic(rabbitConn, ControlTopic, nil)
	if control == nil {
		return nil, nil, fmt.Errorf("Error creating topic")
	}

	var subscriptions []*pubsub.Subscription

	// create a regexp subscription
	if expr != "" {
		// check if it's a valiud regex
		_, err := regexp.Compile(expr)
		if err != nil {
			return nil, nil, err
		}

		// send control message to server
		cMsg := &controlMsg{
			Action: "CREATE",
			Regexp: true,
			Topic:  expr,
		}
		mBody, err := json.Marshal(cMsg)
		if err != nil {
			return nil, nil, err
		}
		msg := &pubsub.Message{
			Body: mBody,
		}
		err = control.Send(context.Background(), msg)
		if err != nil {
			return nil, nil, err
		}

		// create the subscription for regexp log messages
		regExpSub := rabbitpubsub.OpenSubscription(rabbitConn, fmt.Sprintf("__TOPIC__%s", cMsg.Topic), nil)
		if regExpSub == nil {
			return nil, nil, fmt.Errorf("Error creating subscription")
		}
		subscriptions = append(subscriptions, regExpSub)
		log.Infof("Added expr to server: %s", expr)
	}

	// create subscriptions to topics
	if len(topics) == 0 {
		return nil, nil, fmt.Errorf("no topics were supplied")
	}
	for _, topic := range topics {
		// send control message to server
		cMsg := &controlMsg{
			Action: "CREATE",
			Regexp: false,
			Topic:  topic,
		}
		mBody, err := json.Marshal(cMsg)
		if err != nil {
			return nil, nil, err
		}
		msg := &pubsub.Message{
			Body: mBody,
		}
		err = control.Send(context.Background(), msg)
		if err != nil {
			return nil, nil, err
		}

		// create the subscription for regexp log messages
		sub := rabbitpubsub.OpenSubscription(rabbitConn, fmt.Sprintf("__TOPIC__%s", topic), nil)
		if sub == nil {
			return nil, nil, fmt.Errorf("Error creating subscription")
		}
		subscriptions = append(subscriptions, sub)
		log.Infof("Added topic to server: %s", topic)
	}

	errChan := make(chan error)
	lc := &LogitClient{
		log:           log,
		ampqConn:      rabbitConn,
		subscriptions: subscriptions,
		control:       control,
		errChan:       errChan,
		stop:          make(chan bool, 1),
	}
	return lc, errChan, nil
}

func (lc *LogitClient) Run() {
	// for each subscrition we launch a go routine to handle the msgs
	lc.log.Info("Waiting for log messages...")
	for _, sub := range lc.subscriptions {
		go func(sub *pubsub.Subscription) {
			for {
				msg, err := sub.Receive(context.Background())
				if err != nil {
					lc.handleError(err)
					break
				}
				go lc.handleMsg(msg)
				if lc.startTime.IsZero() {
					lc.startTime = time.Now()
				}
			}
		}(sub)
	}
	// channel to block on
	for stop := range lc.stop {
		if stop {
			return
		}
	}
}

func (lc *LogitClient) handleMsg(msg *pubsub.Message) {
	/*if lc.startTime.Add(5*time.Second).Unix() > time.Now().Unix() || lc.logCnt > 99 {
		lc.Stop()
	} else {*/
	cnt := atomic.AddUint32(&lc.logCnt, 1)
	lc.log.Infof("[%d] Log: %s\n", cnt, msg.Body)
	msg.Ack()
	//}
}

func (lc *LogitClient) Stop() {
	for _, sub := range lc.subscriptions {
		sub.Shutdown(context.Background())
	}
	lc.ampqConn.Close()
	lc.stop <- true
}

func (lc *LogitClient) handleError(err error) {
	if err != nil {
		lc.errChan <- err
	}
}
