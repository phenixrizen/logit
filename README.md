## Requiremts
- locally installed RabbitMQ with default guest user

## Install Requirements

```shell
$ make install
```

## Architecture
#### RabbitMQ Pub/Sub Fanout
![GitHub Logo](arch.png)
- A producer (P) is the logitd daemon running in server mode.
    - This daemon accepts tcp connections on port 42280.
    - It takes the tcp stream and parses the logs and adds them to a queue.
- A exchange (X) is the based on regexp or topic
- A queue (amq.gen-*) is a buffer in RabbitMQ that stores messages.
- A consumer (C) is the logitd daemon running in client mode.
    - This daemon registers a queue for consumption of a exchange
    - The client then receives log messages form the queue.

## Build

```shell
$ make
```

## Run

```shell
$ ./logitd server
```

```shell
$ ./logitd client --expr "^Hello.*" --topics queries,purges
```

```shell
$ ./logitd client --topics queries
```

```shell
$ netcat localhost 42280 < logs
```

## Monitor Performane

#### Enable RabbitMQ
```
$ make plug
```

#### Monitor Messgae Rates
- Open [http://localhost:15672/#/](http://localhost:15672/#/) in a browser login as guest.
- View the overview, exchange, queues tabs for message rate details