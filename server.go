package logit

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"regexp"
	"strings"

	"github.com/intwinelabs/logger"
	"github.com/streadway/amqp"
	"gocloud.dev/pubsub"
	"gocloud.dev/pubsub/rabbitpubsub"
)

var ControlTopic = "__CONTROL__"

type LogitServer struct {
	log        *logger.Logger
	listener   *net.TCPListener
	ampqConn   *amqp.Connection
	ampqChan   *amqp.Channel
	control    *pubsub.Topic
	controlSub *pubsub.Subscription
	topics     map[string]*pubsub.Topic
	regexps    map[*regexp.Regexp]*pubsub.Topic
	errChan    chan error
	timeout    int
	stop       chan bool
}

type controlMsg struct {
	Action string `json:"action"`
	Regexp bool   `json:"regexp"`
	Topic  string `json:"topic"`
}

func NewServer(listen string, timeout int, log *logger.Logger) (*LogitServer, chan error, error) {
	// create a listen address
	tcpListenAddr, err := net.ResolveTCPAddr("tcp4", listen)
	if err != nil {
		return nil, nil, err
	}

	// create a tcp listener
	listener, err := net.ListenTCP("tcp", tcpListenAddr)
	if err != nil {
		return nil, nil, err
	}

	// create rabbit mq conn
	rabbitConn, err := amqp.Dial("amqp://guest:guest@localhost:5672/")
	if err != nil {
		return nil, nil, err
	}
	rabbitChan, err := rabbitConn.Channel()
	if err != nil {
		return nil, nil, err
	}

	// create a control topic
	declareTopic(ControlTopic, rabbitChan)
	control := rabbitpubsub.OpenTopic(rabbitConn, ControlTopic, nil)
	if control == nil {
		return nil, nil, fmt.Errorf("Error creating topic")
	}

	// Create a subscription connected to the control topic
	controlSub := rabbitpubsub.OpenSubscription(rabbitConn, ControlTopic, nil)
	if controlSub == nil {
		return nil, nil, fmt.Errorf("Error creating subscription")
	}

	errChan := make(chan error)
	ls := &LogitServer{
		log:        log,
		listener:   listener,
		ampqConn:   rabbitConn,
		ampqChan:   rabbitChan,
		control:    control,
		controlSub: controlSub,
		topics:     make(map[string]*pubsub.Topic),
		regexps:    make(map[*regexp.Regexp]*pubsub.Topic),
		errChan:    errChan,
	}

	return ls, errChan, nil
}

func declareTopic(topic string, rabbitChan *amqp.Channel) error {
	// create a rabbit exchange
	err := rabbitChan.ExchangeDeclare(topic, "fanout", false, false, false, false, nil)
	if err != nil {
		return err
	}
	// create a rabbit queue
	q, err := rabbitChan.QueueDeclare(topic, false, false, false, false, nil)
	if err != nil {
		return err
	}
	err = rabbitChan.QueueBind(q.Name, "", topic, false, nil)
	if err != nil {
		return err
	}
	return nil
}

func (ls *LogitServer) handleConnection(conn net.Conn) {
	ls.log.Infof("Handling connection: %s", conn.RemoteAddr().String())
	// set a read deadline
	//deadline := time.Now().Add(time.Duration(ls.timeout) * time.Minute)
	//err := conn.SetReadDeadline(deadline)
	//ls.handleError(err)

	// create a scanner to handle incoming log lines on the tcp conn
	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		ls.log.Infof("Log Line: %s", scanner.Text())
		go ls.handleLogLine(scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		ls.handleError(err)
	}
	conn.Write([]byte("OK"))
	conn.Close()
}

// "Hello Frank", greetings\n
func (ls *LogitServer) handleLogLine(logLine string) {
	// parse the log line
	idx := strings.LastIndex(logLine, ",")
	var log string
	var top string
	if idx > 2 && len(logLine) > 4 {
		log = logLine[1 : idx-1]
		top = logLine[idx+2:]
		if len(log) > 0 && len(top) > 1 {
			// create a msg
			msg := &pubsub.Message{
				Body: []byte(log),
			}
			// send the msg to the topic
			if topic, ok := ls.topics[top]; ok {
				topic.Send(context.Background(), msg)
			} else {
				// if we have regexp registered we check them
				for regExp, regExpTopic := range ls.regexps {
					// if we have a match we send the msg
					if regExp.Match([]byte(log)) {
						regExpTopic.Send(context.Background(), msg)
					}
				}
			}
		} else {
			ls.errChan <- fmt.Errorf("log line is empty: %s", logLine)
		}
	} else {
		ls.errChan <- fmt.Errorf("Cannot parse log line: %s", logLine)
	}
}

func (ls *LogitServer) handleError(err error) {
	if err != nil {
		ls.errChan <- err
	}
}

func (ls *LogitServer) Run() {
	// handle incoming tcp connections
	go func() {
		for {
			conn, err := ls.listener.Accept()
			if err != nil {
				ls.errChan <- err
			}
			go ls.handleConnection(conn)
		}
	}()

	// handle control msgs
	go ls.handleControlMsgs()

	// channel to block on
	for stop := range ls.stop {
		if stop {
			return
		}
	}

}

func (ls *LogitServer) handleControlMsgs() {
	// we handle control msgs to create to topics
	for {
		msg, err := ls.controlSub.Receive(context.Background())
		if err != nil {
			ls.handleError(err)
			continue
		}
		var cMsg controlMsg
		err = json.Unmarshal(msg.Body, &cMsg)
		if err != nil {
			ls.errChan <- err
			continue
		}
		ls.log.Infof("Handling control message: %+v", cMsg)
		if cMsg.Action == "CREATE" {
			if cMsg.Regexp {
				// create a regexp topic
				regExp, err := regexp.Compile(cMsg.Topic)
				if err != nil {
					ls.errChan <- err
					continue
				}
				err = declareTopic(fmt.Sprintf("__TOPIC__%s", cMsg.Topic), ls.ampqChan)
				if err != nil {
					ls.errChan <- err
					continue
				}
				topic := rabbitpubsub.OpenTopic(ls.ampqConn, fmt.Sprintf("__TOPIC__%s", cMsg.Topic), nil)
				if topic == nil {
					ls.errChan <- fmt.Errorf("Error createing topic: %s", cMsg.Topic)
					continue
				}
				ls.regexps[regExp] = topic
			} else {
				// create a named topic
				err := declareTopic(fmt.Sprintf("__TOPIC__%s", cMsg.Topic), ls.ampqChan)
				if err != nil {
					ls.errChan <- err
					continue
				}
				topic := rabbitpubsub.OpenTopic(ls.ampqConn, fmt.Sprintf("__TOPIC__%s", cMsg.Topic), nil)
				if topic == nil {
					ls.errChan <- fmt.Errorf("Error createing topic: %s", cMsg.Topic)
					continue
				}
				ls.topics[cMsg.Topic] = topic
			}
		}
		msg.Ack()
	}
}

func (ls *LogitServer) Stop() {
	ls.listener.Close()
	for _, topic := range ls.topics {
		topic.Shutdown(context.Background())
	}
	for _, topic := range ls.regexps {
		topic.Shutdown(context.Background())
	}
	ls.ampqConn.Close()
	ls.stop <- true
}
