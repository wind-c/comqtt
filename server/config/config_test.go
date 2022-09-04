package config

import (
	"fmt"
	"github.com/stretchr/testify/require"
	"testing"
)

var buf = []byte(`
cluster:
  mode: false   #true or false
  bindport: 7946
  members:   #seeds member list, format such as 192.168.0.103:7946,192.168.0.104:7946
  queuedepth: 10240 #size of Memberlist's internal channel which handles UDP messages.

mqtt:
  tcp: :1883
  ws: :1882
  http: :8080

redis:
  addr: 127.0.0.1:6739
  password:
  db: 0

log:
  enable: true
  env: 0  #0 dev or 1 prod
  infofile: co-info.log
  errorfile: co-error.log
  maxsize: 100      #100M
  maxage: 30        #30day
  maxbackups: 10    #number of log files
  localtime: true   #true or false
  compress:  true   #true or false
  sampler:
    burst: 3
    period: 1       #second
`)

var file  = "conf.yml"

func TestLoadConfigFromNilFile(t *testing.T) {
	_, err := Load("")
	require.Error(t, err)
}

func TestLoadConfigFromFile(t *testing.T) {
	cfg, err := Load(file)
	require.NoError(t, err)
	require.Equal(t, ":1883", cfg.Mqtt.TCP)
	require.Equal(t, 7946, cfg.Cluster.BindPort)
	require.Equal(t, "127.0.0.1:6739", cfg.Redis.Addr)
	require.Equal(t, 10240, cfg.Cluster.QueueDepth)
	require.Equal(t, 3, cfg.Log.Sampler.Burst)

	fmt.Println(cfg)
}

func TestParse(t *testing.T) {
	cfg, err := parse(buf)
	require.NoError(t, err)
	require.Equal(t, ":1883", cfg.Mqtt.TCP)
	require.Equal(t, 7946, cfg.Cluster.BindPort)
	require.Equal(t, "127.0.0.1:6739", cfg.Redis.Addr)
	require.Equal(t, 10240, cfg.Cluster.QueueDepth)
	require.Equal(t, 3, cfg.Log.Sampler.Burst)
}

