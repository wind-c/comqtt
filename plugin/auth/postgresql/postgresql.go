package postgresql

import (
	"fmt"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"github.com/wind-c/comqtt/plugin"
)

type config struct {
	Host          string `json:"host" yaml:"host"`
	Port          int    `json:"port" yaml:"port"`
	Schema        string `json:"schema" yaml:"schema"`
	SslMode       string `json:"sslmode" yaml:"sslmode"`
	LoginName     string `json:"login-name" yaml:"login-name"`
	LoginPassword string `json:"login-password" yaml:"login-password"`
	MaxOpenConns  int    `json:"max-open-conns" yaml:"max-open-conns"`
	MaxIdleConns  int    `json:"max-idle-conns" yaml:"max-idle-conns"`
	Auth          auth   `json:"auth" yaml:"auth"`
	Acl           acl    `json:"acl" yaml:"acl"`
}

type auth struct {
	Table          string `json:"table" yaml:"table"`
	UserColumn     string `json:"user-column" yaml:"user-column"`
	PasswordColumn string `json:"password-column" yaml:"password-column"`
}

type acl struct {
	Table        string `json:"table" yaml:"table"`
	UserColumn   string `json:"user-column" yaml:"user-column"`
	TopicColumn  string `json:"topic-column" yaml:"topic-column"`
	AccessColumn string `json:"access-column" yaml:"access-column"`
	Publish      string `json:"publish" yaml:"publish"`
	Subscribe    string `json:"subscribe" yaml:"subscribe"`
	PubSub       string `json:"pubsub" yaml:"pubsub"`
}

// Auth is an auth controller which allows access to all connections and topics.
type Auth struct {
	conf     config
	db       *sqlx.DB
	authStmt *sqlx.Stmt
	aclStmt  *sqlx.Stmt
}

func New(confFile string) (*Auth, error) {
	conf := config{}
	err := plugin.LoadYaml(confFile, &conf)
	if err != nil {
		return nil, err
	}
	return &Auth{
		conf: conf,
	}, nil
}

func (a *Auth) Open() error {
	dsn := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		a.conf.Host, a.conf.Port, a.conf.LoginName, a.conf.LoginPassword, a.conf.Schema, a.conf.SslMode)
	sqlxDB, err := sqlx.Connect("postgres", dsn)
	if err != nil {
		return err
	}
	sqlxDB.SetMaxOpenConns(a.conf.MaxOpenConns)
	sqlxDB.SetMaxIdleConns(a.conf.MaxIdleConns)

	authSql := fmt.Sprintf("select %s from %s where %s=? and %s=?",
		a.conf.Auth.UserColumn, a.conf.Auth.Table, a.conf.Auth.UserColumn, a.conf.Auth.PasswordColumn)
	aclSql := fmt.Sprintf("select %s from %s where %s=? and %s=?",
		a.conf.Acl.AccessColumn, a.conf.Acl.Table, a.conf.Acl.UserColumn, a.conf.Acl.TopicColumn)
	a.authStmt, _ = sqlxDB.Preparex(authSql)
	a.aclStmt, _ = sqlxDB.Preparex(aclSql)
	a.db = sqlxDB
	return nil
}

// Close closes the redis instance.
func (a *Auth) Close() {
	a.authStmt.Close()
	a.aclStmt.Close()
	a.db.Close()
}

// Authenticate returns true if a username and password are acceptable. Allow always
// returns true.
func (a *Auth) Authenticate(user, password []byte) bool {
	var username string
	err := a.authStmt.QueryRowx(string(user), string(password)).Scan(&username)
	if err != nil {
		return false
	}
	if username != "" {
		return true
	} else {
		return false
	}
}

// ACL returns true if a user has access permissions to read or write on a topic.
// Allow always returns true.
func (a *Auth) ACL(user []byte, topic string, write bool) bool {
	var access string
	err := a.aclStmt.QueryRowx(string(user), topic).Scan(&access)
	if err != nil {
		return false
	}
	if access == a.conf.Acl.PubSub {
		return true
	} else if access == a.conf.Acl.Publish && write { //publish
		return true
	} else if access == a.conf.Acl.Subscribe && !write { //subscribe
		return true
	} else {
		return false
	}
	return true
}
