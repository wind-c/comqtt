package cluster

import (
	"github.com/hashicorp/logutils"
	"github.com/hashicorp/memberlist"
	"io"
	"net"
	"os"
	"strings"
	"time"
)

const (
	LogLevelDebug = "DEBUG"
	LogLevelWarn  = "WARN"
	LogLevelError = "ERROR"
	LogLevelInfo  = "INFO"
)

type Option func(conf *memberlist.Config)

func NewOptions(opts ...Option) *memberlist.Config {
	conf := memberlist.DefaultLANConfig() // or memberlist.DefaultLocalConfig()
	for _, o := range opts {
		o(conf)
	}
	return conf
}

// WithNodeName the name of this node. This must be unique in the cluster.
func WithNodeName(name string) Option {
	return func(conf *memberlist.Config) {
		conf.Name = name
	}
}

// WithBindAddr "" default "0.0.0.0"
func WithBindAddr(bindAddr string) Option {
	return func(conf *memberlist.Config) {
		conf.BindAddr = bindAddr
	}
}

// WithBindPort 0 dynamically bind a port
func WithBindPort(bindPort int) Option {
	return func(conf *memberlist.Config) {
		conf.BindPort = bindPort
	}
}

// WithAdvertiseAddr "" default "0.0.0.0"
func WithAdvertiseAddr(advertiseAddr string) Option {
	return func(conf *memberlist.Config) {
		conf.AdvertiseAddr = advertiseAddr
	}
}

// WithAdvertisePort 0 dynamically bind a port
func WithAdvertisePort(advertisePort int) Option {
	return func(conf *memberlist.Config) {
		conf.AdvertisePort = advertisePort
	}
}

func WithHandoffQueueDepth(depth int) Option {
	return func(conf *memberlist.Config) {
		if depth != 0 {
			conf.HandoffQueueDepth = depth
		}
	}
}

func WithPushPullInterval(interval int) Option {
	return func(conf *memberlist.Config) {
		conf.PushPullInterval = time.Duration(interval) * time.Second
	}
}

func WithSecretKey(secretKey []byte) Option {
	return func(conf *memberlist.Config) {
		conf.SecretKey = secretKey
	}
}

func WithDelegate(delegate memberlist.Delegate) Option {
	return func(conf *memberlist.Config) {
		conf.Delegate = delegate
	}
}

func WithEvent(event memberlist.EventDelegate) Option {
	return func(conf *memberlist.Config) {
		conf.Events = event
	}
}

func WithLogOutput(writer io.Writer, level string) Option {
	return func(conf *memberlist.Config) {
		if writer == nil {
			writer = os.Stderr
		}
		filter := &logutils.LevelFilter{
			Levels:   []logutils.LogLevel{LogLevelDebug, LogLevelWarn, LogLevelError, LogLevelInfo},
			MinLevel: logutils.LogLevel(strings.ToUpper(level)),
			Writer:   writer,
		}
		conf.LogOutput = filter
	}
}

func WithCIDRsAllowed(ips []net.IPNet) Option {
	return func(conf *memberlist.Config) {
		conf.CIDRsAllowed = ips
	}
}
