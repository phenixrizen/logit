## Requiremts
- locally installed rabbitmq with default guest user

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