package config

import (
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

// Config holds the application configuration loaded from .sbdb.toml and env vars.
type Config struct {
	SchemaDir      string               `mapstructure:"schema_dir"`
	BasePath       string               `mapstructure:"base_path"`
	DefaultSchema  string               `mapstructure:"default_schema"`
	Output         OutputConfig         `mapstructure:"output"`
	Integrity      IntegrityConfig      `mapstructure:"integrity"`
	KnowledgeGraph KnowledgeGraphConfig `mapstructure:"knowledge_graph"`
	Events         EventsConfig         `mapstructure:"events"`
}

// EventsConfig controls the append-only event log.
type EventsConfig struct {
	Enabled           bool          `mapstructure:"enabled"`
	WindowMonths      int           `mapstructure:"window_months"`
	RotationLines     int           `mapstructure:"rotation_lines"`
	MaxLineBytes      int           `mapstructure:"max_line_bytes"`
	AllowNonPosix     bool          `mapstructure:"allow_non_posix"`
	EmitSearchQueried bool          `mapstructure:"emit_search_queried"`
	Archive           ArchiveConfig `mapstructure:"archive"`
}

// ArchiveConfig controls how expired event months are archived.
type ArchiveConfig struct {
	Target          string          `mapstructure:"target"` // "git" | "s3" | "both"
	GzipLevel       int             `mapstructure:"gzip_level"`
	SettleDays      int             `mapstructure:"settle_days"`
	MaxArchiveBytes int64           `mapstructure:"max_archive_bytes"` // refuse archives larger than this
	S3              S3ArchiveConfig `mapstructure:"s3"`
}

// S3ArchiveConfig holds S3 archival settings (used when target = "s3" or "both").
type S3ArchiveConfig struct {
	Bucket       string `mapstructure:"bucket"`
	Prefix       string `mapstructure:"prefix"`
	Region       string `mapstructure:"region"`
	StorageClass string `mapstructure:"storage_class"`
	KMSKeyID     string `mapstructure:"kms_key_id"`
	SSE          string `mapstructure:"sse"`
	Auth         string `mapstructure:"auth"` // env | profile | instance | irsa
	Profile      string `mapstructure:"profile"`
	EndpointURL  string `mapstructure:"endpoint_url"`
	ObjectLock   bool   `mapstructure:"object_lock"`
}

// KnowledgeGraphConfig controls the SQLite knowledge graph and semantic search.
type KnowledgeGraphConfig struct {
	Enabled    bool             `mapstructure:"enabled"`
	DBPath     string           `mapstructure:"db_path"` // relative to base_path
	Embeddings EmbeddingsConfig `mapstructure:"embeddings"`
	Graph      GraphConfig      `mapstructure:"graph"`
}

// EmbeddingsConfig controls the embedding provider.
type EmbeddingsConfig struct {
	Provider  string `mapstructure:"provider"` // "openai", "ollama", "custom"
	BaseURL   string `mapstructure:"base_url"`
	Model     string `mapstructure:"model"`
	Dimension int    `mapstructure:"dimension"`
	BatchSize int    `mapstructure:"batch_size"`
}

// GraphConfig controls knowledge graph behavior.
type GraphConfig struct {
	AutoIndex    bool `mapstructure:"auto_index"`    // index on every save()
	ExtractLinks bool `mapstructure:"extract_links"` // auto-extract markdown links
	ValidateRefs bool `mapstructure:"validate_refs"` // check ref targets exist
}

// OutputConfig controls output formatting.
type OutputConfig struct {
	Format string `mapstructure:"format"` // "auto", "json", "yaml", "table"
}

// IntegrityConfig controls integrity settings.
type IntegrityConfig struct {
	KeySource string `mapstructure:"key_source"` // "env", "file", "keyring"
}

// Load reads configuration from .sbdb.toml in the given directory, falling back to env vars.
func Load(basePath string) (*Config, error) {
	v := viper.New()

	// Defaults
	v.SetDefault("schema_dir", "./schemas")
	v.SetDefault("base_path", ".")
	v.SetDefault("default_schema", "")
	v.SetDefault("output.format", "auto")
	v.SetDefault("integrity.key_source", "env")
	v.SetDefault("knowledge_graph.enabled", false)
	v.SetDefault("knowledge_graph.db_path", "data/.sbdb.db")
	v.SetDefault("knowledge_graph.embeddings.provider", "openai")
	v.SetDefault("knowledge_graph.embeddings.model", "text-embedding-3-small")
	v.SetDefault("knowledge_graph.embeddings.dimension", 1536)
	v.SetDefault("knowledge_graph.embeddings.batch_size", 100)
	v.SetDefault("knowledge_graph.graph.auto_index", true)
	v.SetDefault("knowledge_graph.graph.extract_links", true)
	v.SetDefault("knowledge_graph.graph.validate_refs", false)
	v.SetDefault("events.enabled", false)
	v.SetDefault("events.window_months", 2)
	v.SetDefault("events.rotation_lines", 5000)
	v.SetDefault("events.max_line_bytes", 4096)
	v.SetDefault("events.allow_non_posix", false)
	v.SetDefault("events.emit_search_queried", false)
	v.SetDefault("events.archive.target", "git")
	v.SetDefault("events.archive.gzip_level", 9)
	v.SetDefault("events.archive.settle_days", 7)
	v.SetDefault("events.archive.max_archive_bytes", 1<<30) // 1 GiB
	v.SetDefault("events.archive.s3.storage_class", "STANDARD_IA")
	v.SetDefault("events.archive.s3.sse", "AES256")
	v.SetDefault("events.archive.s3.auth", "env")

	// Config file
	v.SetConfigName(".sbdb")
	v.SetConfigType("toml")
	v.AddConfigPath(basePath)
	v.AddConfigPath(".")

	// Env vars
	v.SetEnvPrefix("SBDB")
	v.AutomaticEnv()

	// Read config (ignore "not found" error)
	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, err
		}
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, err
	}

	// Resolve relative paths
	if !filepath.IsAbs(cfg.SchemaDir) {
		cfg.SchemaDir = filepath.Join(basePath, cfg.SchemaDir)
	}
	if !filepath.IsAbs(cfg.BasePath) {
		cfg.BasePath = basePath
	}

	return &cfg, nil
}

// ResolveFormat picks the output format based on config and TTY detection.
func ResolveFormat(format string) string {
	if format == "" || format == "auto" {
		if isTerminal() {
			return "table"
		}
		return "json"
	}
	return format
}

func isTerminal() bool {
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}
