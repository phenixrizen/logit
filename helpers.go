package logit

import (
	"github.com/streadway/amqp"
)

func declareTopic(topic string, rabbitConn *amqp.Connection) error {
	// create a rabbit Channel
	rabbitChan, err := rabbitConn.Channel()
	if err != nil {
		return err
	}
	// create a rabbit exchange
	err = rabbitChan.ExchangeDeclare(topic, "fanout", false, false, false, false, nil)
	if err != nil {
		return err
	}
	return nil
}

func publishTopic(topic string, rabbitConn *amqp.Connection) (*amqp.Channel, error) {
	// create a rabbit Channel
	return rabbitConn.Channel()
}

func consumeTopic(topic string, rabbitConn *amqp.Connection) (<-chan amqp.Delivery, error) {
	// create a rabbit Channel
	rabbitChan, err := rabbitConn.Channel()
	if err != nil {
		return nil, err
	}
	// create a rabbit queue
	q, err := rabbitChan.QueueDeclare("", false, false, false, false, nil)
	if err != nil {
		return nil, err
	}
	err = rabbitChan.QueueBind(q.Name, "", topic, false, nil)
	if err != nil {
		return nil, err
	}

	return rabbitChan.Consume(q.Name, "", true, false, false, false, nil)
}
