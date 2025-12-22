// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2022 wind
// SPDX-FileContributor: wind (573966@qq.com)

package config

import (
	tls2 "crypto/tls"
	"crypto/x509"
	"errors"
	"os"

	"github.com/wind-c/comqtt/v2/cluster/log"
	comqtt "github.com/wind-c/comqtt/v2/mqtt"
	"gopkg.in/yaml.v3"
)

const (
	DiscoveryWaySerf uint = iota
	DiscoveryWayMemberlist
)

const (
	RaftImplHashicorp uint = iota
	RaftImplEtcd
)

const (
	StorageWayMemory uint = iota
	StorageWayBolt
	StorageWayBadger
	StorageWayRedis
)

const (
	AuthModeAnonymous uint = iota
	AuthModeUsername
	AuthModeClientid
)

const (
	AuthDSFree uint = iota
	AuthDSRedis
	AuthDSMysql
	AuthDSPostgresql
	AuthDSHttp
)

const (
	BridgeWayNone uint = iota
	BridgeWayKafka
)

var (
	ErrAuthWay     = errors.New("auth-way is incorrectly configured")
	ErrStorageWay  = errors.New("only redis can be used in cluster mode")
	ErrClusterOpts = errors.New("cluster options must be configured")

	ErrAppendCerts      = errors.New("append ca cert failure")
	ErrMissingCertOrKey = errors.New("missing server certificate or private key files")
)

func New() *Config {
	return &Config{}
}

func Load(yamlFile string) (*Config, error) {
	bs, err := os.ReadFile(yamlFile)
	if err != nil {
		return nil, err
	}
	return parse(bs)
}

func parse(buf []byte) (*Config, error) {
	conf := &Config{}
	err := yaml.Unmarshal(buf, conf)
	if err != nil {
		return nil, err
	}
	return conf, nil
}

type Config struct {
	StorageWay  uint        `yaml:"storage-way"`
	StoragePath string      `yaml:"storage-path"`
	BridgeWay   uint        `yaml:"bridge-way"`
	BridgePath  string      `yaml:"bridge-path"`
	Auth        auth        `yaml:"auth"`
	Mqtt        mqtt        `yaml:"mqtt"`
	Cluster     Cluster     `yaml:"cluster"`
	Redis       redis       `yaml:"redis"`
	Log         log.Options `yaml:"log"`
	PprofEnable bool        `yaml:"pprof-enable"`
}

type auth struct {
	Way           uint   `yaml:"way"`
	Datasource    uint   `yaml:"datasource"`
	ConfPath      string `yaml:"conf-path"`
	BlacklistPath string `yaml:"blacklist-path"`
}

type mqtt struct {
	TCP     string         `yaml:"tcp"`
	WS      string         `yaml:"ws"`
	HTTP    string         `yaml:"http"`
	Tls     tls            `yaml:"tls"`
	Options comqtt.Options `yaml:"options"`
}

type tls struct {
	CACert     string `yaml:"ca-cert"`
	ServerCert string `yaml:"server-cert"`
	ServerKey  string `yaml:"server-key"`
}

type redisOptions struct {
	Addr     string `json:"addr" yaml:"addr"`
	Username string `json:"username" yaml:"username"`
	Password string `json:"password" yaml:"password"`
	DB       int    `json:"db" yaml:"db"`
}

type redis struct {
	HPrefix string `json:"prefix" yaml:"prefix"`
	Options redisOptions
}

type Cluster struct {
	DiscoveryWay         uint                     `yaml:"discovery-way"  json:"discovery-way"`
	NodeName             string                   `yaml:"node-name" json:"node-name"`
	BindAddr             string                   `yaml:"bind-addr" json:"bind-addr"`
	BindPort             int                      `yaml:"bind-port" json:"bind-port"`
	AdvertiseAddr        string                   `yaml:"advertise-addr" json:"advertise-addr"`
	AdvertisePort        int                      `yaml:"advertise-port" json:"advertise-port"`
	Members              []string                 `yaml:"members" json:"members"`
	DynamicMembership    DynamicMembershipOptions `yaml:"dynamic-membership" json:"dynamic-membership"`
	QueueDepth           int                      `yaml:"queue-depth" json:"queue-depth"`
	Tags                 map[string]string        `yaml:"tags" json:"tags"`
	RaftImpl             uint                     `yaml:"raft-impl" json:"raft-impl"`
	RaftPort             int                      `yaml:"raft-port" json:"raft-port"`
	RaftDir              string                   `yaml:"raft-dir" json:"raft-dir"`
	RaftBootstrap        bool                     `yaml:"raft-bootstrap" json:"raft-bootstrap"`
	RaftLogLevel         string                   `yaml:"raft-log-level" json:"raft-log-level"`
	GrpcEnable           bool                     `yaml:"grpc-enable" json:"grpc-enable"`
	GrpcPort             int                      `yaml:"grpc-port" json:"grpc-port"`
	InboundPoolSize      int                      `yaml:"inbound-pool-size" json:"inbound-pool-size"`
	OutboundPoolSize     int                      `yaml:"outbound-pool-size" json:"outbound-pool-size"`
	InoutPoolNonblocking bool                     `yaml:"inout-pool-nonblocking" json:"inout-pool-nonblocking"`
	NodesFileDir         string                   `yaml:"nodes-file-dir" json:"nodes-file-dir"`
}

type DynamicMembershipOptions struct {
	Enable                   bool   `yaml:"enable" json:"enable"`
	NodeNamePrefix           string `yaml:"node-name-prefix" json:"node-name-prefix"`
	NodeNumZeroPad           uint   `yaml:"node-num-zero-pad" json:"node-num-zero-pad"`
	EventLoopIntervalSec     uint   `yaml:"event-loop-interval-sec" json:"event-loop-interval-sec"`
	RedisNodeKeyPrefix       string `yaml:"redis-node-key-prefix" json:"redis-node-key-prefix"`
	RedisNodeKeyExp          uint   `yaml:"redis-node-key-exp" json:"redis-node-key-exp"`
	RedisLockKey             string `yaml:"redis-lock-key" json:"redis-lock-key"`
	RedisLockLoopIntervalSec uint   `yaml:"redis-lock-loop-interval-sec" json:"redis-lock-loop-interval-sec"`
	MaxRedisLockAttempts     uint   `yaml:"max-redis-lock-attempts" json:"max-redis-lock-attempts"`
}

func GenTlsConfig(conf *Config) (*tls2.Config, error) {
	if conf.Mqtt.Tls.ServerKey == "" && conf.Mqtt.Tls.ServerCert == "" {
		return nil, nil
	}

	if conf.Mqtt.Tls.ServerKey == "" || conf.Mqtt.Tls.ServerCert == "" {
		return nil, ErrMissingCertOrKey
	}

	cert, err := tls2.LoadX509KeyPair(conf.Mqtt.Tls.ServerCert, conf.Mqtt.Tls.ServerKey)
	if err != nil {
		return nil, err
	}
	tlsConfig := &tls2.Config{
		MinVersion:   tls2.VersionTLS12,
		Certificates: []tls2.Certificate{cert},
	}

	// enable bidirectional authentication
	if conf.Mqtt.Tls.CACert != "" {
		pem, err := os.ReadFile(conf.Mqtt.Tls.CACert)
		if err != nil {
			return nil, err
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pem) {
			return nil, ErrAppendCerts
		}

		tlsConfig.RootCAs = pool
		tlsConfig.ClientCAs = pool
		tlsConfig.ClientAuth = tls2.RequireAndVerifyClientCert
	}

	return tlsConfig, nil
}
