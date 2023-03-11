package mysql

import (
	"bytes"
	"fmt"
	_ "github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
	"github.com/wind-c/comqtt/mqtt"
	"github.com/wind-c/comqtt/mqtt/hooks/auth"
	"github.com/wind-c/comqtt/mqtt/packets"
	"github.com/wind-c/comqtt/plugin"
	pa "github.com/wind-c/comqtt/plugin/auth"
)

type Options struct {
	pa.Blacklist
	AuthMode byte      `json:"auth-mode" yaml:"auth-mode"`
	AclMode  byte      `json:"acl-mode" yaml:"acl-mode"`
	Dsn      DsnInfo   `json:"dsn" yaml:"dsn"`
	Auth     AuthTable `json:"auth" yaml:"auth"`
	Acl      AclTable  `json:"acl" yaml:"acl"`
}

type DsnInfo struct {
	Host          string `json:"host" yaml:"host"`
	Port          int    `json:"port" yaml:"port"`
	Schema        string `json:"schema" yaml:"schema"`
	Charset       string `json:"charset" yaml:"charset"`
	LoginName     string `json:"login-name" yaml:"login-name"`
	LoginPassword string `json:"login-password" yaml:"login-password"`
	MaxOpenConns  int    `json:"max-open-conns" yaml:"max-open-conns"`
	MaxIdleConns  int    `json:"max-idle-conns" yaml:"max-idle-conns"`
}

type AuthTable struct {
	Table          string `json:"table" yaml:"table"`
	UserColumn     string `json:"user-column" yaml:"user-column"`
	PasswordColumn string `json:"password-column" yaml:"password-column"`
	AllowColumn    string `json:"allow-column" yaml:"allow-column"`
}

type AclTable struct {
	Table        string `json:"table" yaml:"table"`
	UserColumn   string `json:"user-column" yaml:"user-column"`
	TopicColumn  string `json:"topic-column" yaml:"topic-column"`
	AccessColumn string `json:"access-column" yaml:"access-column"`
}

// Auth is an auth controller which allows access to all connections and topics.
type Auth struct {
	mqtt.HookBase
	config   *Options
	db       *sqlx.DB
	authStmt *sqlx.Stmt
	aclStmt  *sqlx.Stmt
}

// ID returns the ID of the hook.
func (a *Auth) ID() string {
	return "auth-mysql"
}

// Provides indicates which hook methods this hook provides.
func (a *Auth) Provides(b byte) bool {
	return bytes.Contains([]byte{
		mqtt.OnConnectAuthenticate,
		mqtt.OnACLCheck,
	}, []byte{b})
}

func (a *Auth) Init(config any) error {
	if _, ok := config.(*Options); config == nil || (!ok && config != nil) {
		return mqtt.ErrInvalidConfigType
	}

	a.config = config.(*Options)
	a.Log.Info().
		Str("host", a.config.Dsn.Host).
		Str("username", a.config.Dsn.LoginName).
		Int("password-len", len(a.config.Dsn.LoginPassword)).
		Str("db", a.config.Dsn.Schema).
		Msg("connecting to mysql")

	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=%s&parseTime=True&loc=UTC",
		a.config.Dsn.LoginName, a.config.Dsn.LoginPassword, a.config.Dsn.Host, a.config.Dsn.Port, a.config.Dsn.Schema, a.config.Dsn.Charset)
	sqlxDB, err := sqlx.Connect("mysql", dsn)
	if err != nil {
		return err
	}
	sqlxDB.SetMaxOpenConns(a.config.Dsn.MaxOpenConns)
	sqlxDB.SetMaxIdleConns(a.config.Dsn.MaxIdleConns)

	authSql := fmt.Sprintf("select %s, %s from %s where %s=?",
		a.config.Auth.PasswordColumn, a.config.Auth.AllowColumn, a.config.Auth.Table, a.config.Auth.UserColumn)
	aclSql := fmt.Sprintf("select %s, %s from %s where %s=?",
		a.config.Acl.TopicColumn, a.config.Acl.AccessColumn, a.config.Acl.Table, a.config.Acl.UserColumn)
	a.authStmt, _ = sqlxDB.Preparex(authSql)
	a.aclStmt, _ = sqlxDB.Preparex(aclSql)
	a.db = sqlxDB
	return nil
}

// Stop closes the mysql connection.
func (a *Auth) Stop() error {
	a.Log.Info().Msg("disconnecting from mysql")
	a.authStmt.Close()
	a.aclStmt.Close()
	return a.db.Close()
}

// OnConnectAuthenticate returns true if the connecting client has rules which provide access
// in the auth ledger.
func (a *Auth) OnConnectAuthenticate(cl *mqtt.Client, pk packets.Packet) bool {
	if a.config.AuthMode == byte(auth.AuthAnonymous) {
		return true
	}

	// check blacklist
	if n, ok := a.config.CheckBLAuth(cl, pk); n >= 0 { // It's on the blacklist
		return ok
	}

	// normal verification
	var key string
	if a.config.AuthMode == byte(auth.AuthUsername) {
		key = string(cl.Properties.Username)
	} else if a.config.AuthMode == byte(auth.AuthClientID) {
		key = cl.ID
	} else {
		return false
	}

	var password string
	var allow int
	err := a.authStmt.QueryRowx(key).Scan(&password, &allow)
	if err != nil || allow == 0 {
		return false
	}

	if password == string(pk.Connect.Password) {
		return true
	} else {
		return false
	}
}

// OnACLCheck returns true if the connecting client has matching read or write access to subscribe
// or publish to a given topic.
func (a *Auth) OnACLCheck(cl *mqtt.Client, topic string, write bool) bool {
	if a.config.AclMode == byte(auth.AuthAnonymous) {
		return true
	}

	// check blacklist
	if n, ok := a.config.CheckBLAcl(cl, topic, write); n >= 0 { // It's on the blacklist
		return ok
	}

	// normal verification
	var key string
	if a.config.AclMode == byte(auth.AuthUsername) {
		key = string(cl.Properties.Username)
	} else if a.config.AclMode == byte(auth.AuthClientID) {
		key = cl.ID
	} else {
		return false
	}

	rows, err := a.aclStmt.Query(key)
	if err != nil {
		return false
	}

	fam := make(map[string]auth.Access)
	for rows.Next() {
		var filter string
		var access byte
		if err := rows.Scan(&filter, &access); err != nil {
			continue
		}

		if !plugin.MatchTopic(filter, topic) {
			continue
		}

		fam[filter] = auth.Access(access)
	}

	return pa.CheckAcl(fam, write)
}
