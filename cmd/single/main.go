// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2022 wind
// SPDX-FileContributor: wind (573966@qq.com)

package main

import (
	"flag"
	rv8 "github.com/go-redis/redis/v8"
	"github.com/rs/zerolog"
	colog "github.com/wind-c/comqtt/cluster/log/zero"
	"github.com/wind-c/comqtt/config"
	"github.com/wind-c/comqtt/mqtt"
	"github.com/wind-c/comqtt/mqtt/hooks/auth"
	"github.com/wind-c/comqtt/mqtt/hooks/storage/badger"
	"github.com/wind-c/comqtt/mqtt/hooks/storage/bolt"
	"github.com/wind-c/comqtt/mqtt/hooks/storage/redis"
	"github.com/wind-c/comqtt/mqtt/listeners"
	"github.com/wind-c/comqtt/plugin"
	hauth "github.com/wind-c/comqtt/plugin/auth/http"
	mauth "github.com/wind-c/comqtt/plugin/auth/mysql"
	pauth "github.com/wind-c/comqtt/plugin/auth/postgresql"
	rauth "github.com/wind-c/comqtt/plugin/auth/redis"
	cokafka "github.com/wind-c/comqtt/plugin/bridge/kafka"
	"go.etcd.io/bbolt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"
)

var logger *zerolog.Logger

func main() {
	var err error
	var confFile string
	cfg := config.New()

	flag.StringVar(&confFile, "conf", "", "read the program parameters from the config file")
	flag.UintVar(&cfg.StorageWay, "storage-way", 1, "storage way optional items:0 memory, 1 bolt, 2 badger, 3 redis")
	flag.UintVar(&cfg.Auth.Way, "auth-way", 0, "authentication way optional items:0 anonymous, 1 username and password, 2 clientid")
	flag.UintVar(&cfg.Auth.Datasource, "auth-ds", 0, "authentication datasource optional items:0 free, 1 redis, 2 mysql, 3 postgresql, 4 http")
	flag.StringVar(&cfg.Auth.ConfPath, "auth-path", "", "config file path should correspond to the auth-datasource")
	flag.StringVar(&cfg.Mqtt.TCP, "tcp", ":1883", "network address for Mqtt TCP listener")
	flag.StringVar(&cfg.Mqtt.WS, "ws", ":1882", "network address for Mqtt Websocket listener")
	flag.StringVar(&cfg.Mqtt.HTTP, "http", ":8080", "network address for web info dashboard listener")
	flag.BoolVar(&cfg.Log.Enable, "log-enable", true, "log enabled or not")
	flag.IntVar(&cfg.Log.Env, "env", 0, "app running environment，0 development or 1 production")
	flag.IntVar(&cfg.Log.Level, "level", 1, "log level options:0Debug,1Info, 2Warn, 3Error, 4Fatal, 5Panic, 6NoLevel, 7Off")
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

	//init log
	if hn, err := os.Hostname(); err == nil {
		cfg.Log.NodeName = hn
	}
	logger = colog.Init(cfg.Log)
	if cfg.Log.Enable && cfg.Log.Format == 1 {
		log.Println("log output to the files, please check")
	}

	sigs := make(chan os.Signal, 1)
	done := make(chan bool, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigs
		done <- true
	}()

	// create server instance and init hooks
	cfg.Mqtt.Options.Logger = logger
	server := mqtt.New(&cfg.Mqtt.Options)
	logger.Info().Msg("comqtt server initializing...")
	initStorage(server, cfg)
	initAuth(server, cfg)
	initBridge(server, cfg)

	// gen tls config
	var listenerConfig *listeners.Config
	if tlsConfig, err := config.GenTlsConfig(cfg); err != nil {
		server.Log.Fatal().Err(err)
	} else {
		if tlsConfig != nil {
			listenerConfig = &listeners.Config{TLSConfig: tlsConfig}
		}
	}

	// add tcp listener
	tcp := listeners.NewTCP("tcp", cfg.Mqtt.TCP, listenerConfig)
	onError(server.AddListener(tcp), "add tcp listener")

	// add websocket listener
	ws := listeners.NewWebsocket("ws", cfg.Mqtt.WS, listenerConfig)
	onError(server.AddListener(ws), "add websocket listener")

	// add http listener
	http := listeners.NewHTTPStats("stats", cfg.Mqtt.HTTP, nil, server.Info)
	onError(server.AddListener(http), "add http listener")

	// start server
	go func() {
		err := server.Serve()
		if err != nil {
			log.Fatal(err)
		}
	}()
	
	if cfg.Log.Format == 1 {
		log.Println("comqtt server started")
	}

	<-done
	server.Log.Warn().Msg("caught signal, stopping...")
	server.Close()
	server.Log.Info().Msg("main.go finished")
	colog.Close()
}

func initAuth(server *mqtt.Server, conf *config.Config) {
	logMsg := "init auth"
	if conf.Auth.Way == config.AuthModeAnonymous {
		server.AddHook(new(auth.AllowHook), nil)
	} else if conf.Auth.Way == config.AuthModeUsername || conf.Auth.Way == config.AuthModeClientid {
		switch conf.Auth.Datasource {
		case config.AuthDSRedis:
			opts := rauth.Options{}
			onError(plugin.LoadYaml(conf.Auth.ConfPath, &opts), logMsg)
			onError(server.AddHook(new(rauth.Auth), &opts), logMsg)
		case config.AuthDSMysql:
			opts := mauth.Options{}
			onError(plugin.LoadYaml(conf.Auth.ConfPath, &opts), logMsg)
			onError(server.AddHook(new(mauth.Auth), &opts), logMsg)
		case config.AuthDSPostgresql:
			opts := pauth.Options{}
			onError(plugin.LoadYaml(conf.Auth.ConfPath, &opts), logMsg)
			onError(server.AddHook(new(pauth.Auth), &opts), logMsg)
		case config.AuthDSHttp:
			opts := hauth.Options{}
			onError(plugin.LoadYaml(conf.Auth.ConfPath, &opts), logMsg)
			onError(server.AddHook(new(hauth.Auth), &opts), logMsg)
		}
	} else {
		onError(config.ErrAuthWay, logMsg)
	}
}

func initStorage(server *mqtt.Server, conf *config.Config) {
	logMsg := "init storage"
	switch conf.StorageWay {
	case config.StorageWayBolt:
		onError(server.AddHook(new(bolt.Hook), &bolt.Options{
			Path: conf.StoragePath,
			Options: &bbolt.Options{
				Timeout: 500 * time.Millisecond,
			},
		}), logMsg)
	case config.StorageWayBadger:
		onError(server.AddHook(new(badger.Hook), &badger.Options{
			Path: conf.StoragePath,
		}), logMsg)
	case config.StorageWayRedis:
		onError(server.AddHook(new(redis.Hook), &redis.Options{
			HPrefix: conf.Redis.HPrefix,
			Options: &rv8.Options{
				Addr:     conf.Redis.Options.Addr,
				DB:       conf.Redis.Options.DB,
				Password: conf.Redis.Options.Password,
			},
		}), logMsg)
	}
}

func initBridge(server *mqtt.Server, conf *config.Config) {
	logMsg := "init bridge"
	if conf.BridgeWay == config.BridgeWayNone {
		return
	} else if conf.BridgeWay == config.BridgeWayKafka {
		opts := cokafka.Options{}
		onError(plugin.LoadYaml(conf.BridgePath, &opts), logMsg)
		onError(server.AddHook(new(cokafka.Bridge), &opts), logMsg)
	}
}

// onError handle errors and simplify code
func onError(err error, msg string) {
	if err != nil {
		logger.Fatal().Err(err).Msg(msg)
	}
}
