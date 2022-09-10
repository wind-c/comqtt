package main

import (
	"flag"
	"fmt"
	"github.com/go-redis/redis/v8"
	"github.com/logrusorgru/aurora"
	iauth "github.com/wind-c/comqtt/plugin/auth"
	rauth "github.com/wind-c/comqtt/plugin/auth/redis"
	mqtt "github.com/wind-c/comqtt/server"
	cs "github.com/wind-c/comqtt/server/cluster"
	red "github.com/wind-c/comqtt/server/cluster/persistence/redis"
	"github.com/wind-c/comqtt/server/config"
	"github.com/wind-c/comqtt/server/listeners"
	colog "github.com/wind-c/comqtt/server/log"
	"github.com/wind-c/comqtt/server/persistence/bolt"
	"go.etcd.io/bbolt"
	"log"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"syscall"
	"time"
)

var server *mqtt.Server
var cluster *cs.Cluster

func main() {
	//go http.ListenAndServe(":9990", nil)
	var err error
	var confFile string
	cfg := config.New()

	flag.StringVar(&confFile, "conf", "", "read the program parameters from the config file")
	flag.UintVar(&cfg.RunMode, "mode", 1, "optional value 1 single or 2 cluster mode")
	flag.BoolVar(&cfg.RedisStorage, "redis-storage", false, "Whether to enable redis storage in cluster mode")
	flag.BoolVar(&cfg.FileStorage, "file-storage", false, "whether to enable file storage in single-node mode")
	flag.StringVar(&cfg.Mqtt.TCP, "tcp", ":1883", "network address for TCP listener")
	flag.StringVar(&cfg.Mqtt.WS, "ws", ":1882", "network address for Websocket listener")
	flag.StringVar(&cfg.Mqtt.HTTP, "http", ":8080", "network address for web info dashboard listener")
	flag.IntVar(&cfg.Mqtt.ReceiveMaximum, "receive-maximum", 512, "the maximum number of QOS1 & 2 messages allowed to be 'inflight'")
	flag.IntVar(&cfg.Cluster.BindPort, "gossip-port", 0, "listening port for cluster node,if this parameter is not set,then port is dynamically bound")
	flag.IntVar(&cfg.Cluster.RaftPort, "raft-port", 0, "listening port for raft node,if this parameter is not set,then port is dynamically bound")
	flag.StringVar(&cfg.Cluster.Members, "members", "", "seeds member list of cluster,such as 192.168.0.103:7946,192.168.0.104:7946")
	flag.StringVar(&cfg.Redis.Addr, "redis", "localhost:6379", "redis address for cluster mode")
	flag.StringVar(&cfg.Redis.Password, "redis-pass", "", "redis password for cluster mode")
	flag.IntVar(&cfg.Redis.DB, "redis-db", 0, "redis db for cluster mode")
	flag.BoolVar(&cfg.Log.Enable, "enable", false, "log enabled or not")
	flag.IntVar(&cfg.Log.Env, "env", 0, "app running environment，0 development or 1 production")
	flag.IntVar(&cfg.Log.Level, "level", 1, "log level")
	flag.StringVar(&cfg.Log.InfoFile, "info-file", "./logs/co-info.log", "info log filename")
	flag.StringVar(&cfg.Log.ErrorFile, "error-file", "./logs/co-err.log", "error log filename")
	//parse arguments
	flag.Parse()
	//load config file
	if confFile != "" {
		if cfg, err = config.Load(confFile); err != nil {
			log.Fatal(err)
		}
	}
	mqttCfg := cfg.Mqtt
	redisCfg := cfg.Redis
	csCfg := cfg.Cluster
	logCfg := cfg.Log

	//init zap log
	if hn, err := os.Hostname(); err == nil {
		logCfg.NodeName = hn
	}
	colog.Init(logCfg)

	//listen system operations
	sigs := make(chan os.Signal, 1)
	done := make(chan bool, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigs
		done <- true
	}()

	fmt.Println(aurora.Magenta("CoMQTT Broker initializing..."))
	fmt.Println(aurora.Cyan("TCP"), mqttCfg.TCP)
	fmt.Println(aurora.Cyan("Websocket"), mqttCfg.WS)
	fmt.Println(aurora.Cyan("$SYS Dashboard"), mqttCfg.HTTP)

	// server options...
	if cfg.RunMode == 0 { // The command line and configuration file are not configured
		cfg.RunMode = mqtt.Cluster
	}
	options := &mqtt.Options{
		RunMode:          cfg.RunMode,             // Program running mode，1 single or 2 cluster
		BufferSize:       mqttCfg.BufferSize,      // Use default values 1024 * 256
		BufferBlockSize:  mqttCfg.BufferBlockSize, // Use default values 1024 * 8
		ReceiveMaximum:   mqttCfg.ReceiveMaximum,
		InflightHandling: mqttCfg.InflightHandling,
	}
	server := mqtt.NewServer(options)

	// Add store
	if cfg.RunMode == mqtt.Cluster {
		if cfg.RedisStorage {
			err = server.AddStore(red.New(&redis.Options{
				Addr:     redisCfg.Addr,
				Password: redisCfg.Password, // no password set
				DB:       redisCfg.DB,       // use default DB
			}))
			if err != nil {
				log.Fatal(err)
			}
		}
	} else {
		if cfg.FileStorage {
			err = server.AddStore(bolt.New("comqtt.db", &bbolt.Options{
				Timeout: 500 * time.Millisecond,
			}))
			if err != nil {
				log.Fatal(err)
			}
		}
	}

	var auth iauth.Auth
	if cfg.AuthDatasource == "redis" {
		auth, err = rauth.New("./plugin/auth/redis/conf.yml")
		if err != nil {
			log.Fatal(err)
		}
		err = auth.Open()
		if err != nil {
			log.Fatal(err)
		}
		defer auth.Close()
	}

	tcp := listeners.NewT("t1", mqttCfg.TCP, auth)
	err = server.AddListener(tcp, nil)
	if err != nil {
		log.Fatal(err)
	}

	ws := listeners.NewW("ws1", mqttCfg.WS, auth)
	err = server.AddListener(ws, nil)
	if err != nil {
		log.Fatal(err)
	}

	stats := listeners.NewH("stats", mqttCfg.HTTP, nil)
	err = server.AddListener(stats, nil)
	if err != nil {
		log.Fatal(err)
	}

	go func() {
		err := server.Serve()
		if err != nil {
			log.Fatal(err)
		}
	}()
	fmt.Println(aurora.BgMagenta("Mqtt Server Started!  "))

	if cfg.RunMode == mqtt.Cluster {
		ops := make([]cs.Option, 3)
		ops[0] = cs.WithLogOutput(nil, cs.LogLevelInfo) //Used to filter memberlist logs
		ops[1] = cs.WithBindPort(csCfg.BindPort)
		ops[2] = cs.WithHandoffQueueDepth(csCfg.QueueDepth)
		if csCfg.NodeName != "" {
			ops = append(ops, cs.WithNodeName(csCfg.NodeName))
		}

		cluster, err = cs.LaunchNode(csCfg.Members, ops...)
		if err != nil {
			log.Fatal(err)
		}
		cluster.BindMqttServer(server)

		//bootstrap raft node
		if csCfg.RaftPort == 0 {
			csCfg.RaftPort = 8701
		}
		if csCfg.RaftDir == "" {
			csCfg.RaftDir = "./raft"
		}

		err = cluster.BootstrapRaft(csCfg.RaftPort, csCfg.RaftDir)
		if err != nil {
			log.Fatal(err)
		}

		fmt.Println(aurora.BgMagenta("Cluster Node Created! "))
	}

	<-done
	colog.Sync() //flushing any buffered log entries
	fmt.Println(aurora.BgRed("  Caught Signal  "))

	//server.Close()
	fmt.Println(aurora.BgGreen("  Finished  "))
}
