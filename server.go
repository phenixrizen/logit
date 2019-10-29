package logit

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"regexp"
	"strings"

	"github.com/intwinelabs/logger"
	"github.com/streadway/amqp"
)

var ControlTopic = "__CONTROL__"

type LogitServer struct {
	log        *logger.Logger
	listener   *net.TCPListener
	ampqConn   *amqp.Connection
	control    *amqp.Channel
	controlSub <-chan amqp.Delivery
	topics     map[string]*amqp.Channel
	regexps    map[*regexp.Regexp]*amqp.Channel
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

	// create a control topic
	err = declareTopic(ControlTopic, rabbitConn)
	if err != nil {
		return nil, nil, err
	}
	control, err := publishTopic(ControlTopic, rabbitConn)
	if err != nil {
		return nil, nil, fmt.Errorf("Error creating topic: %s", err)
	}

	// Create a subscription connected to the control topic
	controlSub, err := consumeTopic(ControlTopic, rabbitConn)
	if err != nil {
		return nil, nil, fmt.Errorf("Error creating subscription: %s", err)
	}

	errChan := make(chan error)
	ls := &LogitServer{
		log:        log,
		listener:   listener,
		ampqConn:   rabbitConn,
		control:    control,
		controlSub: controlSub,
		topics:     make(map[string]*amqp.Channel),
		regexps:    make(map[*regexp.Regexp]*amqp.Channel),
		errChan:    errChan,
		stop:       make(chan bool),
	}

	return ls, errChan, nil
}

func (ls *LogitServer) handleConnection(conn net.Conn) {
	ls.log.Infof("Handling connection: %s", conn.RemoteAddr().String())

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
			msg := amqp.Publishing{
				ContentType: "text/plain",
				Body:        []byte(log),
			}
			// send the msg to the topic
			if topic, ok := ls.topics[top]; ok {
				err := topic.Publish(fmt.Sprintf("__TOPIC__%s", top), "", false, false, msg)
				if err != nil {
					ls.errChan <- fmt.Errorf("error sending log line on msg queue: %s", logLine)
				}
			} else {
				// if we have regexp registered we check them
				for regExp, regExpTopic := range ls.regexps {
					// if we have a match we send the msg
					if regExp.Match([]byte(log)) {
						err := regExpTopic.Publish(fmt.Sprintf("__TOPIC__%s", regExp.String()), "", false, false, msg)
						if err != nil {
							ls.errChan <- fmt.Errorf("error sending log line on msg queue: %s", logLine)
						}
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
				continue
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
	for msg := range ls.controlSub {
		var cMsg controlMsg
		err := json.Unmarshal(msg.Body, &cMsg)
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
				err = declareTopic(fmt.Sprintf("__TOPIC__%s", cMsg.Topic), ls.ampqConn)
				if err != nil {
					ls.errChan <- fmt.Errorf("Error createing topic: %s", cMsg.Topic)
					continue
				}
				topic, err := publishTopic(cMsg.Topic, ls.ampqConn)
				if err != nil {
					ls.errChan <- fmt.Errorf("Error createing topic: %s", cMsg.Topic)
					continue
				}
				ls.regexps[regExp] = topic
			} else {
				// create a named topic
				err := declareTopic(fmt.Sprintf("__TOPIC__%s", cMsg.Topic), ls.ampqConn)
				if err != nil {
					ls.errChan <- err
					continue
				}
				topic, err := publishTopic(cMsg.Topic, ls.ampqConn)
				if topic == nil {
					ls.errChan <- fmt.Errorf("Error createing topic: %s", cMsg.Topic)
					continue
				}
				ls.topics[cMsg.Topic] = topic
			}
		}
	}
}

func (ls *LogitServer) Stop() {
	for _, topic := range ls.topics {
		topic.Close()
	}
	for _, topic := range ls.regexps {
		topic.Close()
	}
	ls.listener.Close()
	ls.ampqConn.Close()
	ls.stop <- true
}
