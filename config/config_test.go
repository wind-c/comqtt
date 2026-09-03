// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2022 wind
// SPDX-FileContributor: wind (573966@qq.com)

package config

import (
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

var buf = []byte(`
auth-mode: 1
cluster:
  mode: false   #true or false
  bind-addr: 0.0.0.0
  bind-port: 7946
  advertise-addr: 0.0.0.0
  advertise-port: 7946
  members:   #seeds member list, format such as 192.168.0.103:7946,192.168.0.104:7946
  queue-depth: 10240 #size of Memberlist's internal channel which handles UDP messages.

mqtt:
  tcp: :1883
  ws: :1882
  http: :8080
  options:
    fan-pool-size: 32
    fan-pool-queue-size: 1024
    sys-topic-resend-interval: 1
    capabilities:
      compatibilities:
        obscure-not-authorized: false
      maximum-message-expiry-interval: 86400
      maximum-session-expiry-interval: 4294967295
      maximum-packet-size: 0 
      receive-maximum: 1024

redis:
  options:
    addr: 127.0.0.1:6379
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

var file = "conf.yml"

func TestLoadConfigFromNilFile(t *testing.T) {
	_, err := Load("")
	require.Error(t, err)
}

func TestLoadConfigFromFile(t *testing.T) {
	cfg, err := Load(file)
	require.NoError(t, err)
	require.Equal(t, ":1883", cfg.Mqtt.TCP)
	require.Equal(t, 7946, cfg.Cluster.BindPort)
	require.Equal(t, "127.0.0.1:6379", cfg.Redis.Options.Addr)
	require.Equal(t, 10240, cfg.Cluster.QueueDepth)

	fmt.Println(cfg)
}

func TestParse(t *testing.T) {
	cfg, err := parse(buf)
	require.NoError(t, err)
	require.Equal(t, ":1883", cfg.Mqtt.TCP)
	require.Equal(t, 7946, cfg.Cluster.BindPort)
	require.Equal(t, "127.0.0.1:6379", cfg.Redis.Options.Addr)
	require.Equal(t, 10240, cfg.Cluster.QueueDepth)
}

func TestDashboardConfigDefaults(t *testing.T) {
	cfg := New()
	require.True(t, cfg.DashboardEnable)
	require.Equal(t, "", cfg.Dashboard.SecretFile)
	require.Equal(t, "config/dashboard-users.json", cfg.Dashboard.UsersFile)
}

func TestDashboardConfigFromYaml(t *testing.T) {
	cfg, err := parse([]byte(`
dashboard-enable: true
dashboard:
  secret-file: /etc/comqtt/secret
  users-file: /etc/comqtt/users.json
`))
	require.NoError(t, err)
	require.True(t, cfg.DashboardEnable)
	require.Equal(t, "/etc/comqtt/secret", cfg.Dashboard.SecretFile)
	require.Equal(t, "/etc/comqtt/users.json", cfg.Dashboard.UsersFile)
}

func TestDashboardConfigDisabled(t *testing.T) {
	cfg, err := parse([]byte(`
dashboard-enable: false
dashboard:
  users-file: /etc/comqtt/users.json
`))
	require.NoError(t, err)
	require.False(t, cfg.DashboardEnable)
}

func TestEnvOverrideTopLevelString(t *testing.T) {
	t.Setenv("COMQTT_STORAGE_PATH", "/env/storage.db")
	cfg, err := parse([]byte(`
storage-path: /default/storage.db
`))
	require.NoError(t, err)
	require.Equal(t, "/default/storage.db", cfg.StoragePath)

	applyEnvOverrides(cfg)
	require.Equal(t, "/env/storage.db", cfg.StoragePath)
}

func TestEnvOverrideTopLevelUint(t *testing.T) {
	t.Setenv("COMQTT_STORAGE_WAY", "2")
	cfg, err := parse([]byte(`
storage-way: 1
`))
	require.NoError(t, err)
	require.Equal(t, uint(1), cfg.StorageWay)

	applyEnvOverrides(cfg)
	require.Equal(t, uint(2), cfg.StorageWay)
}

func TestEnvOverrideTopLevelBool(t *testing.T) {
	t.Setenv("COMQTT_PPROF_ENABLE", "true")
	cfg, err := parse([]byte(`
pprof-enable: false
`))
	require.NoError(t, err)
	require.False(t, cfg.PprofEnable)

	applyEnvOverrides(cfg)
	require.True(t, cfg.PprofEnable)
}

func TestEnvOverrideNestedString(t *testing.T) {
	t.Setenv("COMQTT_MQTT_TCP", ":9883")
	cfg, err := parse([]byte(`
mqtt:
  tcp: :1883
`))
	require.NoError(t, err)
	require.Equal(t, ":1883", cfg.Mqtt.TCP)

	applyEnvOverrides(cfg)
	require.Equal(t, ":9883", cfg.Mqtt.TCP)
}

func TestEnvOverrideNestedInt(t *testing.T) {
	t.Setenv("COMQTT_CLUSTER_BIND_PORT", "9946")
	cfg, err := parse([]byte(`
cluster:
  bind-port: 7946
`))
	require.NoError(t, err)
	require.Equal(t, 7946, cfg.Cluster.BindPort)

	applyEnvOverrides(cfg)
	require.Equal(t, 9946, cfg.Cluster.BindPort)
}

func TestEnvOverrideNestedStructString(t *testing.T) {
	t.Setenv("COMQTT_REDIS_OPTIONS_ADDR", "10.0.0.1:6379")
	cfg, err := parse([]byte(`
redis:
  options:
    addr: 127.0.0.1:6379
`))
	require.NoError(t, err)
	require.Equal(t, "127.0.0.1:6379", cfg.Redis.Options.Addr)

	applyEnvOverrides(cfg)
	require.Equal(t, "10.0.0.1:6379", cfg.Redis.Options.Addr)
}

func TestEnvOverrideMultiple(t *testing.T) {
	t.Setenv("COMQTT_MQTT_TCP", ":9883")
	t.Setenv("COMQTT_CLUSTER_BIND_PORT", "9946")
	t.Setenv("COMQTT_STORAGE_WAY", "2")
	t.Setenv("COMQTT_REDIS_OPTIONS_ADDR", "10.0.0.1:6379")

	cfg, err := parse([]byte(`
storage-way: 0
cluster:
  bind-port: 7946
mqtt:
  tcp: :1883
redis:
  options:
    addr: 127.0.0.1:6379
`))
	require.NoError(t, err)
	require.Equal(t, uint(0), cfg.StorageWay)
	require.Equal(t, 7946, cfg.Cluster.BindPort)
	require.Equal(t, ":1883", cfg.Mqtt.TCP)
	require.Equal(t, "127.0.0.1:6379", cfg.Redis.Options.Addr)

	applyEnvOverrides(cfg)
	require.Equal(t, uint(2), cfg.StorageWay)
	require.Equal(t, 9946, cfg.Cluster.BindPort)
	require.Equal(t, ":9883", cfg.Mqtt.TCP)
	require.Equal(t, "10.0.0.1:6379", cfg.Redis.Options.Addr)
}

func TestEnvOverrideEmptyStringClearsValue(t *testing.T) {
	// An env var set to "" explicitly overrides (clears) the YAML value.
	t.Setenv("COMQTT_STORAGE_PATH", "")
	cfg, err := parse([]byte(`
storage-path: /default/storage.db
`))
	require.NoError(t, err)
	require.Equal(t, "/default/storage.db", cfg.StoragePath)

	applyEnvOverrides(cfg)
	require.Equal(t, "", cfg.StoragePath)
}

func TestEnvOverrideDoesNotMutateWhenNoEnv(t *testing.T) {
	cfg, err := parse([]byte(`
storage-way: 1
storage-path: /default/storage.db
mqtt:
  tcp: :1883
`))
	require.NoError(t, err)
	applyEnvOverrides(cfg)
	require.Equal(t, uint(1), cfg.StorageWay)
	require.Equal(t, "/default/storage.db", cfg.StoragePath)
	require.Equal(t, ":1883", cfg.Mqtt.TCP)
}

func TestLoadWithEnvOverride(t *testing.T) {
	// Verify Load integrates env overrides end-to-end.
	// We need a real file for Load; use conf.yml which already exists.
	// Override one value via env.
	t.Setenv("COMQTT_MQTT_TCP", ":19999")
	cfg, err := Load("conf.yml")
	require.NoError(t, err)
	require.Equal(t, ":19999", cfg.Mqtt.TCP)
}

func TestEnvOverrideClusterBool(t *testing.T) {
	t.Setenv("COMQTT_CLUSTER_GRPC_ENABLE", "true")
	cfg, err := parse([]byte(`
cluster:
  grpc-enable: false
`))
	require.NoError(t, err)
	require.False(t, cfg.Cluster.GrpcEnable)

	applyEnvOverrides(cfg)
	require.True(t, cfg.Cluster.GrpcEnable)
}

func TestEnvOverrideDashboardEnable(t *testing.T) {
	t.Setenv("COMQTT_DASHBOARD_ENABLE", "false")
	cfg, err := parse([]byte(`
dashboard-enable: true
`))
	require.NoError(t, err)
	require.True(t, cfg.DashboardEnable)

	applyEnvOverrides(cfg)
	require.False(t, cfg.DashboardEnable)
}

func TestEnvOverrideDashboardSecretFile(t *testing.T) {
	t.Setenv("COMQTT_DASHBOARD_SECRET_FILE", "/run/secrets/comqtt")
	cfg, err := parse([]byte(`
dashboard:
  secret-file: ""
`))
	require.NoError(t, err)
	require.Equal(t, "", cfg.Dashboard.SecretFile)

	applyEnvOverrides(cfg)
	require.Equal(t, "/run/secrets/comqtt", cfg.Dashboard.SecretFile)
}

func TestEnvOverrideUnsetDoesNotClearValue(t *testing.T) {
	// An unset env var must leave the YAML value untouched.
	os.Unsetenv("COMQTT_STORAGE_PATH")
	cfg, err := parse([]byte(`
storage-path: /default/storage.db
`))
	require.NoError(t, err)

	applyEnvOverrides(cfg)
	require.Equal(t, "/default/storage.db", cfg.StoragePath)
}

// Issue #119: Kubernetes pod IP is dynamic, so cluster.bind-addr must be
// overridable via an env var at deploy time without rewriting the YAML.
func TestEnvOverrideIssue119BindAddr(t *testing.T) {
	t.Setenv("COMQTT_CLUSTER_BIND_ADDR", "10.244.0.17")
	t.Setenv("COMQTT_CLUSTER_NODE_NAME", "node01")
	t.Setenv("COMQTT_CLUSTER_RAFT_BOOTSTRAP", "true")

	cfg, err := parse([]byte(`
cluster:
  bind-addr: 0.0.0.0
  node-name: placeholder
  raft-bootstrap: false
  members: [node01.svc:7946, node02.svc:7946]
`))
	require.NoError(t, err)

	applyEnvOverrides(cfg)
	require.Equal(t, "10.244.0.17", cfg.Cluster.BindAddr)
	require.Equal(t, "node01", cfg.Cluster.NodeName)
	require.Equal(t, true, cfg.Cluster.RaftBootstrap)
	// Slices/maps are not overridable via env; members must stay as parsed.
	require.Equal(t, []string{"node01.svc:7946", "node02.svc:7946"}, cfg.Cluster.Members)
}
