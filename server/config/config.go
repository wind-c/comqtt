package config

import (
	"gopkg.in/yaml.v3"
	"os"
)

func New() *config {
	return &config{}
}

func Load(yamlFile string) (*config, error) {
	bs, err := os.ReadFile(yamlFile)
	if err != nil {
		return nil, err
	}
	return parse(bs)
}

func parse(buf []byte) (*config, error) {
	conf := &config{}
	err := yaml.Unmarshal(buf, conf)
	if err != nil {
		return nil, err
	}
	return conf, nil
}

type config struct {
	RunMode        uint    `json:"run-mode" yaml:"run-mode"`
	RedisStorage   bool    `json:"redis-storage" yaml:"redis-storage"`
	FileStorage    bool    `json:"file-storage" yaml:"file-storage"`
	AuthDatasource string  `json:"auth-datasource" yaml:"auth-datasource"`
	Mqtt           mqtt    `json:"mqtt" yaml:"mqtt"`
	Cluster        cluster `json:"cluster" yaml:"cluster"`
	Redis          redis   `json:"redis" yaml:"redis"`
	Log            Log     `json:"log" yaml:"log"`
}

type mqtt struct {
	TCP              string `json:"tcp" yaml:"tcp"`
	WS               string `json:"ws" yaml:"ws"`
	HTTP             string `json:"http" yaml:"http"`
	BufferSize       int    `json:"buffer-size" yaml:"buffer-size"`
	BufferBlockSize  int    `json:"buffer-block-size" yaml:"buffer-block-size"`
	ReceiveMaximum   int    `json:"receive-maximum" yaml:"receive-maximum"`
	InflightHandling int    `json:"inflight-handling" yaml:"inflight-handling"`
}

type redis struct {
	Addr     string `json:"addr" yaml:"addr"`
	Password string `json:"password" yaml:"password"`
	DB       int    `json:"db" yaml:"db"`
}

type cluster struct {
	NodeName      string `json:"node-name" yaml:"node-name"`
	BindAddr      string `json:"bind-addr" yaml:"bind-addr"`
	BindPort      int    `json:"bind-port" yaml:"bind-port"`
	AdvertiseAddr string `json:"advertise-addr" yaml:"advertise-addr"`
	AdvertisePort int    `json:"advertise-port" yaml:"advertise-port"`
	Members       string `json:"members" yaml:"members"`
	QueueDepth    int    `json:"queue-depth" yaml:"queue-depth"`
	RaftPort      int    `json:"raft-port" yaml:"raft-port"`
	RaftDir       string `json:"raft-dir" yaml:"raft-dir"`
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

	// Filename is the file to write logs to.  Backup log files will be retained
	// in the same directory.  It uses <processname>-lumberjack.log in
	// os.TempDir() if empty.
	InfoFile  string `json:"info-file" yaml:"info-file"`
	ErrorFile string `json:"error-file" yaml:"error-file"`

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
