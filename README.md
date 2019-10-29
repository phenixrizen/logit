## Requiremts
- locally installed rabbitmq with default guest user

## Install Requirements

```shell
$ make install
```

## Architecture
![GitHub Logo](arch.png)
- A producer is the logitd daemon running in server mode. 
    - This daemon accepts tcp connections on port 42280.
    - It takes the tcp stream and parses the logs and adds them to a queue.
- A queue is a buffer in rabbitmq that stores messages.
- A consumer is the logitd daemon running in client mode.
    - This daemon receives log messages form the queue.

## Build

```shell
$ make
```

## Run

```shell
$ ./logitd server
$ ./logitd client --expr "^Hello.*" --topics queries,purges
$ ./logitd client --topics queries
$ netcat localhost 42280 < logs
```