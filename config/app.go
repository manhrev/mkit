package config

import (
	"github.com/manhrev/mkit/enum"
	"time"
)

type App struct {
	Environment enum.Environment `validate:"required"`
	Timezone    string           `validate:"required"`
	Version     string           `validate:"required"`
	Postgres    *Postgres
	Redis       *Redis
	HTTP        *HTTP
	GRPC        *GRPC
	Log         *Log
	Tracing     Tracing
	Minio       *Minio
	RabbitMQ    *RabbitMQ
	JWT         *JWT
}

type Postgres struct {
	ConnectionParams *PostgresConnectionParams `validate:"required"`
	IsMigrateSchema  bool
	// IsCockroach migrates with golang-migrate's cockroachdb driver (lock
	// row in schema_lock): CockroachDB has no pg_advisory_lock, which the
	// default pgx driver needs.
	IsCockroach bool
	MaxOpenConn int `validate:"required,gt=0"`
	MaxIdleConn int `validate:"required,gt=0"`
}

type PostgresConnectionParams struct {
	Host     string `validate:"required,hostname|ip"`
	Port     int    `validate:"required,gt=0"`
	User     string `validate:"required"`
	Password string `validate:"required"`
	DBName   string `validate:"required"`
	SSLMode  string `validate:"required,oneof=disable require verify-full"`
}

type Redis struct {
	Host     string `validate:"required,hostname|ip"`
	Port     string
	Password string
	DB       int
}

type GRPC struct {
	Host                 string `validate:"required,hostname|ip"`
	Port                 string `validate:"required,gt=0"`
	MaxConnectionAge     string
	ReflectionEnabled    bool
	JsonTranscodeEnabled bool
}

type HTTP struct {
	Host string `validate:"required,hostname|ip"`
	Port string `validate:"required,gt=0"`
}

type Tracing struct {
	ServiceName   string
	Enabled       bool // master switch; false disables all signals below
	CollectorHost string
	CollectorPort int
	Secure        bool
	Trace         bool
	Metric        bool
	Log           bool
}

type Log struct {
	Level       string `validate:"required,oneof=error warn info debug"`
	LogFilePath string `validate:"required"`
}

type GRPCClient struct {
	Host string
	Port string
}

type Minio struct {
	Host       string `validate:"required,hostname|ip"`
	Port       int    `validate:"required,gt=0"`
	AccessKey  string `validate:"required"`
	SecretKey  string `validate:"required"`
	BucketName string `validate:"required"`
	SSLEnabled bool
}

type RabbitMQ struct {
	Host     string `validate:"required,hostname|ip"`
	Port     int    `validate:"required,gt=0"`
	User     string `validate:"required"`
	Password string `validate:"required"`
}

type JWT struct {
	PrivateKeyFile       string `validate:"required"`
	PublicKeyFile        string `validate:"required"`
	AccessTokenDuration  time.Duration
	RefreshTokenDuration time.Duration
	Issuer               string
}

type Cronjob struct {
	ID          string
	Spec        string
	TaskTimeout time.Duration
	Disabled    bool
}
