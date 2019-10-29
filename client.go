package logit

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sync/atomic"
	"time"

	"github.com/intwinelabs/logger"
	"github.com/streadway/amqp"
)

type LogitClient struct {
	log           *logger.Logger
	ampqConn      *amqp.Connection
	subscriptions []<-chan amqp.Delivery
	control       *amqp.Channel
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
	control, err := publishTopic(ControlTopic, rabbitConn)
	if err != nil {
		return nil, nil, fmt.Errorf("Error opening publishing topic: %s")
	}

	var subscriptions []<-chan amqp.Delivery

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
		msg := amqp.Publishing{
			ContentType: "application/json",
			Body:        []byte(mBody),
		}
		err = control.Publish(ControlTopic, "", false, false, msg)
		if err != nil {
			return nil, nil, fmt.Errorf("error sending control message: %s", mBody)
		}

		// create the subscription for regexp log messages
		regExpSub, err := consumeTopic(fmt.Sprintf("__TOPIC__%s", cMsg.Topic), rabbitConn)
		if err != nil {
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
		msg := amqp.Publishing{
			ContentType: "application/json",
			Body:        []byte(mBody),
		}
		err = control.Publish(ControlTopic, "", false, false, msg)
		if err != nil {
			return nil, nil, fmt.Errorf("error sending control message: %s", mBody)
		}

		// create the subscription for regexp log messages
		sub, err := consumeTopic(fmt.Sprintf("__TOPIC__%s", cMsg.Topic), rabbitConn)
		if err != nil {
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
		go func(sub <-chan amqp.Delivery) {
			for msg := range sub {
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

func (lc *LogitClient) handleMsg(msg amqp.Delivery) {
	cnt := atomic.AddUint32(&lc.logCnt, 1)
	lc.log.Infof("[%d] Log: %s\n", cnt, msg.Body)
}

func (lc *LogitClient) Stop() {
	lc.ampqConn.Close()
	lc.stop <- true
}

func (lc *LogitClient) handleError(err error) {
	if err != nil {
		lc.errChan <- err
	}
}
