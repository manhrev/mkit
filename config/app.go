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
	S3          *S3
	GCS         *GCS
	LocalBucket *LocalBucket
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

// S3 is an S3 API bucket (e.g. Cloudflare R2), see objectstorage/s3store.
// Objects are served publicly from somewhere else (a custom domain), not
// from Endpoint.
type S3 struct {
	Endpoint        string // R2: https://<account_id>.r2.cloudflarestorage.com
	Region          string // R2: auto
	Bucket          string
	AccessKeyID     string
	SecretAccessKey string
}

// GCS is a Google Cloud Storage bucket, see objectstorage/gcsstore. Credentials come from
// CredentialsFile when set, else Application Default Credentials (the
// runtime service account on Cloud Run, `gcloud auth application-default
// login` locally).
type GCS struct {
	BucketName      string `validate:"required"`
	CredentialsFile string
	// SignerEmail is the service account signed URLs are signed as, through
	// the IAM Credentials signBlob API (needs iam.serviceAccounts.signBlob on
	// it). Empty = detect it from the credentials, which works with a key
	// file or on Cloud Run/GCE, not with user ADC.
	SignerEmail string
}

// LocalBucket is a dev-only bucket on local disk, see
// objectstorage/localbucket.
type LocalBucket struct {
	Dir string // where objects are stored, created if missing
	URL string // public URL the API serves them under, e.g. http://localhost:8080/local-bucket/
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
