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
- Cat the logs file for the format I'm expecting

## Monitor Performane

#### Enable RabbitMQ
```
$ make plug
```

#### Monitor Messgae Rates
- Open [http://localhost:15672/#/](http://localhost:15672/#/) in a browser login as guest.
- View the overview, exchange, queues tabs for message rate details

## Sample Output
```shell
$ ./logitd server
INFO : 2019/10/29 09:05:09.590550 server.go:34: Running in server mode....
INFO : 2019/10/29 09:05:09.590625 server.go:35: Send logs to: :42280
INFO : 2019/10/29 09:05:16.404681 server.go:184: Handling control message: {Action:CREATE Regexp:true Topic:^Hello.*}
INFO : 2019/10/29 09:05:16.408845 server.go:184: Handling control message: {Action:CREATE Regexp:false Topic:queries}
INFO : 2019/10/29 09:05:16.413801 server.go:184: Handling control message: {Action:CREATE Regexp:false Topic:purges}
INFO : 2019/10/29 09:05:19.700182 server.go:184: Handling control message: {Action:CREATE Regexp:false Topic:queries}
INFO : 2019/10/29 09:05:23.082226 server.go:88: Handling connection: 127.0.0.1:55490
INFO : 2019/10/29 09:05:23.082323 server.go:93: Log Line: "Hello Frank", greetings
INFO : 2019/10/29 09:05:23.082335 server.go:93: Log Line: "purge to 5", purges
INFO : 2019/10/29 09:05:23.082344 server.go:93: Log Line: "Franky says Hello", junk
INFO : 2019/10/29 09:05:23.082351 server.go:93: Log Line: "Hello Margret", queries
INFO : 2019/10/29 09:05:23.082411 server.go:93: Log Line: "Where am I", queries
INFO : 2019/10/29 09:05:23.082473 server.go:93: Log Line: "Super secret value", secrets
```

```shell
$ ./logitd client --expr "^Hello.*" --topics queries,purges
INFO : 2019/10/29 09:05:16.400337 client.go:34: Running in client mode....
INFO : 2019/10/29 09:05:16.400412 client.go:35: Looking for logs that match: ^Hello.*
INFO : 2019/10/29 09:05:16.400420 client.go:36: Looking for logs on topics: [queries purges]
INFO : 2019/10/29 09:05:16.407795 client.go:73: Added expr to server: ^Hello.*
INFO : 2019/10/29 09:05:16.412894 client.go:106: Added topic to server: queries
INFO : 2019/10/29 09:05:16.417312 client.go:106: Added topic to server: purges
INFO : 2019/10/29 09:05:16.417695 client.go:123: Waiting for log messages...
INFO : 2019/10/29 09:05:23.084235 client.go:144: [1] Log: purge to 5
INFO : 2019/10/29 09:05:23.084324 client.go:144: [2] Log: Hello Margret
INFO : 2019/10/29 09:05:23.084422 client.go:144: [3] Log: Where am I
INFO : 2019/10/29 09:05:23.084509 client.go:144: [4] Log: Hello Frank
```

```shell
$ ./logitd client --topics queries
INFO : 2019/10/29 09:05:19.695682 client.go:34: Running in client mode....
INFO : 2019/10/29 09:05:19.695754 client.go:35: Looking for logs that match: 
INFO : 2019/10/29 09:05:19.695761 client.go:36: Looking for logs on topics: [queries]
INFO : 2019/10/29 09:05:19.704534 client.go:106: Added topic to server: queries
INFO : 2019/10/29 09:05:19.704790 client.go:123: Waiting for log messages...
INFO : 2019/10/29 09:05:23.084351 client.go:144: [1] Log: Hello Margret
INFO : 2019/10/29 09:05:23.084495 client.go:144: [2] Log: Where am I
```