# Comqtt Cluster

The open source Go-implemented mqtt-broker projects on github generally do not support clustering or mqttv5, both of which comqtt supports very well.

Comqtt cluster uses gossip for node discovery and raft for data consistency synchronization. It uses redis to store inflight, retain messages and subscriptions across nodes.

Of course, due to my busy work and lack of more production environment verification, bugs will inevitably occur, so you are welcome to use, raise issues and contribute.

## Cluster Features

- Cluster nodes are automatically discovered using the goosip protocol.
- Subscribe and unsubscribe messages use the raft protocol to synchronize consistency between nodes.
- Publish messages support point-to-point transmission using GRPC, not broadcast to all nodes.
- Both cluser and standalone support bridging messages to kafka, as well as multiple auth&acl ways.
- Horizontal scaling is supported. When adding new nodes, you only need to specify any node in the cluster as the seed node.
- Simple metrics viewing, such as mqtt statistics and cluster statistics.

## Build

```shell
cd cmd
go build -o comqtt cluster/main.go
```

## Usage

```shell
./comqtt -h
Usage of ./comqtt:
  -conf string
        read the program parameters from the config file
        
  -storage-way uint
        storage way options:0 memory, 1 bolt, 2 badger, 3 redis (default 3)
  -auth-ds uint
        authentication datasource options:0 free, 1 redis, 2 mysql, 3 postgresql, 4 http
  -auth-path string
        config file path should correspond to the auth-datasource
  -auth-way uint
        authentication way options:0 anonymous, 1 username and password, 2 clientid

  -node-name string
        node name must be unique in the cluster
  -members string
        seeds member list of cluster,such as 192.168.0.103:7946,192.168.0.104:7946
  -bind-ip string
        the ip used for discovery and communication between nodes. It is usually set to the intranet ip addr. (default "127.0.0.1")
  -gossip-port int
        this port is used to discover nodes in a cluster
  -raft-port int
        this port is used for raft peer communication
  -raft-bootstrap bool
        should be true for the first node of the cluster. It can elect a leader without any other nodes being present. (default false)
  -grpc-enable bool
	    grpc is used for raft transport and reliable communication between nodes. (default false)
  -grpc-port int
        grpc communication port between nodes
        
  -http string
        network address for web info dashboard listener (default ":8080")
  -tcp string
        network address for mqtt tcp listener (default ":1883")
  -ws string
        network address for mqtt websocket listener (default ":1882")

  -redis string
        redis address for cluster mode (default "127.0.0.1:6379")
  -redis-db int
        redis db for cluster mode
  -redis-pass string
        redis password for cluster mode
  
  -log-enable
        log enabled or not (default true)
  -level int
        log level options:0Debug,1Info, 2Warn, 3Error, 4Fatal, 5Panic, 6NoLevel, 7Off (default 1)
  -env int
        app running environment:0 development or 1 production
  -error-file string
        error log filename (default "./logs/co-err.log")
  -info-file string
        info log filename (default "./logs/co-info.log")
```

The startup supports two modes: command parameters and configuration file. 

It is recommended that you use the configuration file mode for more detailed configuration.[Click to config example](../cmd/config/node1.yml).

All configuration file examples are in the cmd/config directory. [Click to config examples](../cmd/config)

### Configure Redis

Start redis and configure redis addr, [click to config example](../cmd/config/node1.yml).

### Create Cluster

*Start three nodes on one laptop*

*If you want to obtain the bridge and multiple authentication capabilities, you need to use the configuration file to start.*

#### 1. Start first node
```shell
./comqtt --node-name=c01 --gossip-port=7946 --raft-port=8946 --raft-bootstrap=true
or
./comqtt --conf=./config/node1.yml
```

You should see the output
```
8:16PM INF comqtt server initializing...
8:16PM INF added hook hook=redis-db
8:16PM INF connecting to redis service address=127.0.0.1:6379 db=0 hook=redis-db
8:16PM INF connected to redis service hook=redis-db
8:16PM INF added hook hook=allow-all-auth
8:16PM INF added hook hook=agent-event
8:16PM INF found raft leader leader=c01
8:16PM INF setup raft addr=127.0.0.1:8946 node=c01
8:16PM INF local member addr=127.0.0.1 port=7946
8:16PM INF cluster node created
8:16PM INF attached listener address=:1883 id=tcp protocol=tcp
8:16PM INF attached listener address=:1882 id=ws protocol=ws
8:16PM INF attached listener address=:8080 id=stats protocol=http
8:16PM INF comqtt server started
8:16PM INF notify join addr=127.0.0.1 node=c02
8:16PM INF raft joined addr=127.0.0.1:8947 node=c02
8:17PM INF notify join addr=127.0.0.1 node=c03
8:17PM INF raft joined addr=127.0.0.1:8948 node=c03
```

#### 2. Start second node with first node as part of the member list
```shell
./comqtt --node-name=c02 --gossip-port=7947 --raft-port=8947 --members=localhost:7946 --tcp=:1885 --ws=:1886 --http=:1881
or
./comqtt --conf=./config/node2.yml
```

You should see the output
```
8:16PM INF comqtt server initializing...
8:16PM INF added hook hook=redis-db
8:16PM INF connecting to redis service address=127.0.0.1:6379 db=0 hook=redis-db
8:16PM INF connected to redis service hook=redis-db
8:16PM INF added hook hook=allow-all-auth
8:16PM INF added hook hook=agent-event
8:16PM INF setup raft addr=127.0.0.1:8947 node=c02
8:16PM INF local member addr=127.0.0.1 port=7947
8:16PM INF cluster node created
8:16PM INF attached listener address=:1885 id=tcp protocol=tcp
8:16PM INF attached listener address=:1886 id=ws protocol=ws
8:16PM INF attached listener address=:1881 id=stats protocol=http
8:16PM INF notify join addr=127.0.0.1 node=c01
8:16PM INF raft joining addr=127.0.0.1:8946 node=c01
8:16PM INF comqtt server started
8:17PM INF notify join addr=127.0.0.1 node=c03
8:17PM INF raft joining addr=127.0.0.1:8948 node=c03
```

First node output will log the new connection
```shell
2022/03/17 00:20:44 [DEBUG] memberlist: Stream connection from=192.168.0.103:49756
2022/03/17 00:20:45 [DEBUG] memberlist: Initiating push/pull sync with: 18d26675-d04f-4114-bea2-163e7a68a219 192.168.0.103:7947
```
#### 3. Start third node with first node as part of the member list
```shell
./comqtt --node-name=c03 --gossip-port=7948 --raft-port=8948 --members=localhost:7946 --tcp=:1887 --ws=:1888 --http=:1882
or
./comqtt --conf=./config/node3.yml
```

### Performance (messages/second)

```shell
mqtt-stresser -broker tcp://localhost:1883 -num-clients=500 -num-messages=100
```
Start two nodes on one laptop and test at the same time.

A laptop：MacBook Pro (13-inch, M1, 16G).

#### 1. First Node

*Publishing Throughput*

Fastest: 156996 msg/sec

Slowest: 1115 msg/sec

Median: 6219 msg/sec

*Receiving Througput*

Fastest: 1460728 msg/sec

Slowest: 1146 msg/sec

Median: 6584 msg/sec

####2. Second Node

*Publishing Throughput*

Fastest: 190537 msg/sec

Slowest: 932 msg/sec

Median: 3406 msg/sec

*Receiving Througput*

Fastest: 1647229 msg/sec

Slowest: 1241 msg/sec

Median: 4951 msg/sec
