// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2022 wind
// SPDX-FileContributor: wind (573966@qq.com)

package config

import (
	tls2 "crypto/tls"
	"crypto/x509"
	"errors"
	comqtt "github.com/wind-c/comqtt/v2/mqtt"
	"gopkg.in/yaml.v3"
	"os"
)

const (
	DiscoveryWaySerf uint = iota
	DiscoveryWayMemberlist
	DiscoveryWayMDNS
	DiscoveryWayStatic
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
	StorageWay  uint    `yaml:"storage-way"`
	StoragePath string  `yaml:"storage-path"`
	BridgeWay   uint    `yaml:"bridge-way"`
	BridgePath  string  `yaml:"bridge-path"`
	Auth        auth    `yaml:"auth"`
	Mqtt        mqtt    `yaml:"mqtt"`
	Cluster     Cluster `yaml:"cluster"`
	Redis       redis   `yaml:"redis"`
	Log         Log     `yaml:"log"`
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
	DiscoveryWay  uint              `yaml:"discovery-way"  json:"discovery-way"`
	NodeName      string            `yaml:"node-name" json:"node-name"`
	BindAddr      string            `yaml:"bind-addr" json:"bind-addr"`
	BindPort      int               `yaml:"bind-port" json:"bind-port"`
	AdvertiseAddr string            `yaml:"advertise-addr" json:"advertise-addr"`
	AdvertisePort int               `yaml:"advertise-port" json:"advertise-port"`
	Members       []string          `yaml:"members" json:"members"`
	QueueDepth    int               `yaml:"queue-depth" json:"queue-depth"`
	Tags          map[string]string `yaml:"tags" json:"tags"`
	RaftImpl      uint              `yaml:"raft-impl" json:"raft-impl"`
	RaftPort      int               `yaml:"raft-port" json:"raft-port"`
	RaftDir       string            `yaml:"raft-dir" json:"raft-dir"`
	RaftBootstrap bool              `yaml:"raft-bootstrap" json:"raft-bootstrap"`
	GrpcEnable    bool              `yaml:"grpc-enable" json:"grpc-enable"`
	GrpcPort      int               `yaml:"grpc-port" json:"grpc-port"`
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

type Log struct {
	//　Enable Log enabled or not
	Enable bool `json:"enable" yaml:"enable"`

	// Env app running environment，0 development or 1 production
	Env int `json:"env" yaml:"env"`

	// NodeName used in a cluster environment to distinguish nodes
	NodeName string `json:"node-name" yaml:"node-name"`

	// Format output format 0 console or 1 json
	Format int `json:"format" yaml:"format"`

	// Whether to display code line number
	Caller bool `json:"caller" yaml:"caller"`

	// Filename is the file to write logs to.  Backup log files will be retained
	// in the same directory.  It uses <processname>-lumberjack.log in
	// os.TempDir() if empty.
	InfoFile       string `json:"info-file" yaml:"info-file"`
	ErrorFile      string `json:"error-file" yaml:"error-file"`
	ThirdpartyFile string `json:"thirdparty-file" yaml:"thirdparty-file"`

	// MaxSize is the maximum size in megabytes of the log file before it gets
	// rotated. It defaults to 100 megabytes.
	MaxSize int `json:"maxsize" yaml:"maxsize"`

	// MaxAge is the maximum number of days to retain old log files based on the
	// timestamp encoded in their filename.  Note that a day is defined as 24
	// hours and may not exactly correspond to calendar days due to daylight
	// savings, leap seconds, etc. The default is not to remove old log files
	// based on age.
	MaxAge int `json:"max-age" yaml:"max-age"`

	// MaxBackups is the maximum number of old log files to retain.  The default
	// is to retain all old log files (though MaxAge may still cause them to get
	// deleted.)
	MaxBackups int `json:"max-backups" yaml:"max-backups"`

	// LocalTime determines if the time used for formatting the timestamps in
	// backup files is the computer's local time.  The default is to use UTC
	// time.
	Localtime bool `json:"localtime" yaml:"localtime"`

	// Compress determines if the rotated log files should be compressed
	// using gzip. The default is not to perform compression.
	Compress bool `json:"compress" yaml:"compress"`
	// contains filtered or unexported fields

	// Log Level: -1Trace 0Debug 1Info 2Warn 3Error(default) 4Fatal 5Panic 6NoLevel 7Off
	Level int `json:"level" yaml:"level"`
	Sampler
}

type Sampler struct {
	Burst  int `json:"burst" yaml:"burst"`
	Period int `json:"period" yaml:"period"`
}
