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
	GCS         *GCS
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
	// SkipPing makes New return without connecting, so a caller can bind its
	// HTTP port first and ping later. The caller must then ping itself and
	// treat failure as fatal; otherwise a bad DSN only shows on first query.
	SkipPing    bool
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
	ServiceName string
	Enabled     bool // master switch; false disables all signals below
	// Endpoint is the OTLP/gRPC collector URL for all signals; the scheme
	// picks TLS: http://localhost:4317 (plaintext) or https://host:443.
	Endpoint string
	Headers  map[string]string // sent on every export, e.g. Authorization
	Trace    bool
	Metric   bool
	Log      bool
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

// GCS is a Google Cloud Storage default bucket. Credentials come from
// CredentialsFile when set, else Application Default Credentials (the
// runtime service account on Cloud Run, `gcloud auth application-default
// login` locally).
type GCS struct {
	BucketName      string `validate:"required"`
	CredentialsFile string
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
