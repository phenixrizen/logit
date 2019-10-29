package logit

import (
	"encoding/json"
	"net"
	"os"
	"testing"
	"time"

	"github.com/intwinelabs/logger"
	"github.com/streadway/amqp"
	"github.com/stretchr/testify/assert"
)

func TestService(t *testing.T) {
	a := assert.New(t)

	logFile := "testing.log"
	os.Remove(logFile)
	lf, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0660)
	a.Nil(err, "error should be nil")
	defer lf.Close()

	log := logger.Init("testing", false, false, lf)

	// setup server
	server, serverErrChan, err := NewServer(":42281", 2, log)
	a.Nil(err, "error should be nil")
	a.NotNil(server, "server should not be nil")
	a.NotNil(serverErrChan, "err channel should not be nil")

	// run the server
	go server.Run()

	// listen to control channel
	controlSub, err := consumeTopic(ControlTopic, server.ampqConn)
	a.Nil(err, "error should be nil")

	// setup client
	client, clientErrChan, err := NewClient("^Testing*", []string{"testing", "test"}, log)
	a.Nil(err, "error should be nil")
	a.NotNil(client, "client should be nil")
	a.NotNil(clientErrChan, "err channel should  be nil")

	// test control messages from client to server
	msgs := make(chan controlMsg, 3)
	a.NotNil(msgs)
	go func(msgs chan controlMsg) {
		for msg := range controlSub {
			var cMsg controlMsg
			err := json.Unmarshal(msg.Body, &cMsg)
			log.Info(cMsg)
			a.Nil(err, "error decoding json control message")
			msgs <- cMsg
		}
	}(msgs)

	msgCnt := 0
	for msg := range msgs {
		log.Info(msgCnt, msg)
		if msgCnt == 0 {
			a.Equal(controlMsg{Action: "CREATE", Regexp: true, Topic: "^Testing*"}, msg, "control message is not correct")
			msgCnt++
			continue
		}
		if msgCnt == 1 {
			a.Equal(controlMsg{Action: "CREATE", Regexp: false, Topic: "testing"}, msg, "control message is not correct")
			msgCnt++
			continue
		}
		if msgCnt == 2 {
			a.Equal(controlMsg{Action: "CREATE", Regexp: false, Topic: "test"}, msg, "control message is not correct")
			msgCnt++
			break
		}
	}

	// test log messages from server to client
	// listen to ^Tesing* regexp topic channel
	regexpChannel, err := consumeTopic("__TOPIC__^Testing*", server.ampqConn)
	a.Nil(err, "error should be nil")
	logMsgs := make(chan amqp.Delivery, 1)
	a.NotNil(logMsgs)
	go func(msgs chan amqp.Delivery) {
		for msg := range regexpChannel {
			log.Info(msg.Exchange)
			logMsgs <- msg
		}
	}(logMsgs)

	// send data to server via a tcp socket
	logData := "\"Testing 1 2 3 Frank\", foo\n\"purge to 5\", testing\n\"Test it\", test\n"
	ingestAddr, err := net.ResolveTCPAddr("tcp4", ":42281")
	a.Nil(err, "error should be nil")
	ingestConn, err := net.DialTCP("tcp", nil, ingestAddr)
	a.Nil(err, "error should be nil")
	n, err := ingestConn.Write([]byte(logData))
	a.Equal(len(logData), n, "improper number of bytes written to socket")

	msgCnt = 0
	for lm := range logMsgs {
		if msgCnt == 0 {
			a.Equal([]uint8{0x54, 0x65, 0x73, 0x74, 0x69, 0x6e, 0x67, 0x20, 0x31, 0x20, 0x32, 0x20, 0x33, 0x20, 0x46, 0x72, 0x61, 0x6e, 0x6b}, lm.Body, "log message is not correct")
			msgCnt++
			break
		}
	}

	_, err = ingestConn.Write([]byte(logData))
	tm := time.Now().Unix()
	go client.Run()
	a.True(tm > client.startTime.Unix())
	lf.Sync()

	// stop the client and server
	client.Stop()
	server.Stop()
}
