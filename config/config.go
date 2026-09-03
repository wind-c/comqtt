// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2022 wind
// SPDX-FileContributor: wind (573966@qq.com)

package config

import (
	"crypto/ed25519"
	"crypto/rand"
	tls2 "crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"

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
	return &Config{
		DashboardEnable: true,
		Dashboard: DashboardConfig{
			UsersFile: "config/dashboard-users.json",
		},
	}
}

func Load(yamlFile string) (*Config, error) {
	bs, err := os.ReadFile(yamlFile)
	if err != nil {
		return nil, err
	}
	conf, err := parse(bs)
	if err != nil {
		return nil, err
	}
	applyEnvOverrides(conf)
	return conf, nil
}

func parse(buf []byte) (*Config, error) {
	conf := &Config{}
	err := yaml.Unmarshal(buf, conf)
	if err != nil {
		return nil, err
	}
	return conf, nil
}

// envPrefix is the prefix for all environment variable overrides.
const envPrefix = "COMQTT_"

// applyEnvOverrides walks the Config struct and overrides fields whose
// corresponding environment variables are set.
//
// Env var naming: the key is the dotted yaml path of the field, with the
// top-level "COMQTT_" prefix, each segment uppercased and any '-' replaced
// by '_', and segments joined by '_' (not doubled). For example:
//
//	cluster.bind-addr      -> COMQTT_CLUSTER_BIND_ADDR
//	mqtt.tcp               -> COMQTT_MQTT_TCP
//	redis.options.addr     -> COMQTT_REDIS_OPTIONS_ADDR
//	dashboard.secret-file  -> COMQTT_DASHBOARD_SECRET_FILE
//
// Only scalar fields are overridden (string, uint/int, bool). Composite
// fields such as slices and maps are not settable via env vars and are
// silently left to their YAML value. A field is overridden only when the
// env var is set; an empty string explicitly clears the YAML value.
func applyEnvOverrides(conf *Config) {
	applyEnv(reflect.ValueOf(conf).Elem(), envPrefix)
}

func applyEnv(v reflect.Value, prefix string) {
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		fieldVal := v.Field(i)

		if !field.IsExported() {
			continue
		}

		tag := field.Tag.Get("yaml")
		if tag == "" || tag == "-" {
			continue
		}
		// Strip any comma-separated options (e.g. "yaml,omitempty").
		if idx := strings.IndexByte(tag, ','); idx != -1 {
			tag = tag[:idx]
		}

		envKey := prefix + strings.ToUpper(strings.ReplaceAll(tag, "-", "_"))

		switch fieldVal.Kind() {
		case reflect.Struct:
			applyEnv(fieldVal, envKey+"_")
		case reflect.String:
			if val, ok := os.LookupEnv(envKey); ok {
				fieldVal.SetString(val)
			}
		case reflect.Uint:
			if val, ok := os.LookupEnv(envKey); ok {
				if n, err := strconv.ParseUint(val, 10, 64); err == nil {
					fieldVal.SetUint(n)
				}
			}
		case reflect.Int:
			if val, ok := os.LookupEnv(envKey); ok {
				if n, err := strconv.ParseInt(val, 10, 64); err == nil {
					fieldVal.SetInt(n)
				}
			}
		case reflect.Bool:
			if val, ok := os.LookupEnv(envKey); ok {
				if b, err := strconv.ParseBool(val); err == nil {
					fieldVal.SetBool(b)
				}
			}
		}
	}
}

type Config struct {
	StorageWay      uint            `yaml:"storage-way"`
	StoragePath     string          `yaml:"storage-path"`
	BridgeWay       uint            `yaml:"bridge-way"`
	BridgePath      string          `yaml:"bridge-path"`
	Auth            auth            `yaml:"auth"`
	Mqtt            mqtt            `yaml:"mqtt"`
	Cluster         Cluster         `yaml:"cluster"`
	Redis           redis           `yaml:"redis"`
	Log             log.Options     `yaml:"log"`
	PprofEnable     bool            `yaml:"pprof-enable"`
	DashboardEnable bool            `yaml:"dashboard-enable"`
	Dashboard       DashboardConfig `yaml:"dashboard"`
}

type DashboardConfig struct {
	SecretFile string `yaml:"secret-file"`
	UsersFile  string `yaml:"users-file"`
}

type auth struct {
	Way           uint   `yaml:"way"`
	Datasource    uint   `yaml:"datasource"`
	ConfPath      string `yaml:"conf-path"`
	BlacklistPath string `yaml:"blacklist-path"`
}

type mqtt struct {
	TCP     string         `yaml:"tcp"`
	QUIC    string         `yaml:"quic"`
	WS      string         `yaml:"ws"`
	HTTP    string         `yaml:"http"`
	Tls     tls            `yaml:"tls"`
	Options comqtt.Options `yaml:"options"`
}

type tls struct {
	CACert     string `yaml:"ca-cert"`
	ServerCert string `yaml:"server-cert"`
	ServerKey  string `yaml:"server-key"`
	ZeroRTT    bool   `yaml:"zero-rtt"`
}

type redisOptions struct {
	Addr     string `json:"addr"     yaml:"addr"`
	Username string `json:"username" yaml:"username"`
	Password string `json:"password" yaml:"password"`
	DB       int    `json:"db"       yaml:"db"`
}

type redis struct {
	HPrefix string       `json:"prefix" yaml:"prefix"`
	Options redisOptions `yaml:"options"`
}

type Cluster struct {
	DiscoveryWay         uint              `yaml:"discovery-way"          json:"discovery-way"`
	NodeName             string            `yaml:"node-name"              json:"node-name"`
	BindAddr             string            `yaml:"bind-addr"              json:"bind-addr"`
	BindPort             int               `yaml:"bind-port"              json:"bind-port"`
	AdvertiseAddr        string            `yaml:"advertise-addr"         json:"advertise-addr"`
	AdvertisePort        int               `yaml:"advertise-port"         json:"advertise-port"`
	Members              []string          `yaml:"members"                json:"members"`
	QueueDepth           int               `yaml:"queue-depth"            json:"queue-depth"`
	Tags                 map[string]string `yaml:"tags"                   json:"tags"`
	RaftImpl             uint              `yaml:"raft-impl"              json:"raft-impl"`
	RaftPort             int               `yaml:"raft-port"              json:"raft-port"`
	RaftDir              string            `yaml:"raft-dir"               json:"raft-dir"`
	RaftBootstrap        bool              `yaml:"raft-bootstrap"         json:"raft-bootstrap"`
	RaftLogLevel         string            `yaml:"raft-log-level"         json:"raft-log-level"`
	GrpcEnable           bool              `yaml:"grpc-enable"            json:"grpc-enable"`
	GrpcPort             int               `yaml:"grpc-port"              json:"grpc-port"`
	InboundPoolSize      int               `yaml:"inbound-pool-size"      json:"inbound-pool-size"`
	OutboundPoolSize     int               `yaml:"outbound-pool-size"     json:"outbound-pool-size"`
	InoutPoolNonblocking bool              `yaml:"inout-pool-nonblocking" json:"inout-pool-nonblocking"`
	NodesFileDir         string            `yaml:"nodes-file-dir"         json:"nodes-file-dir"`
	HttpPort             int               `yaml:"http-port"              json:"http-port"`
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

func GenerateSelfSignedCert() (*tls2.Config, error) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}

	serial, err := rand.Int(rand.Reader, big.NewInt(1<<62))
	if err != nil {
		return nil, err
	}

	template := x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			Organization: []string{"MQTT Auto Generated"},
		},
		NotBefore: time.Now().Add(-time.Hour),
		NotAfter:  time.Now().Add(365 * 24 * time.Hour),

		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,

		DNSNames: []string{"localhost"},
	}

	derBytes, err := x509.CreateCertificate(
		rand.Reader,
		&template,
		&template,
		priv.Public(),
		priv,
	)
	if err != nil {
		return nil, err
	}

	certPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: derBytes,
	})

	keyBytes, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return nil, err
	}

	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: keyBytes,
	})

	cert, err := tls2.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, err
	}
	tlsConfig := &tls2.Config{
		MinVersion:   tls2.VersionTLS13,
		Certificates: []tls2.Certificate{cert},
		NextProtos:   []string{"mqtt"},
	}

	return tlsConfig, nil
}
