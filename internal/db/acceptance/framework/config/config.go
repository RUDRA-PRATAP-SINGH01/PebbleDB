// Package config manages configuration loading, validation, and priority overriding
// from defaults, custom environment files, environment variables, and CLI parameters.
//
// Dependency Rules:
// - Imports: types, errors.
package config

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db/acceptance/framework/errors"
	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db/acceptance/framework/types"
)

// Default settings
const (
	DefaultMemtableSize        = 4 << 20 // 4MB
	DefaultCompactionThreshold = 4
	DefaultSyncWrites          = false
	DefaultParallelism         = 1
	DefaultMaxRetries          = 3
	DefaultTimeout             = 30 * time.Second
)

// ConfigLoader manages loading, parsing and merging configuration options.
type ConfigLoader struct {
	baseDirOverride string
	cliArgs         []string
}

// NewConfigLoader allocates a ConfigLoader.
func NewConfigLoader(baseDirOverride string, cliArgs []string) *ConfigLoader {
	return &ConfigLoader{
		baseDirOverride: baseDirOverride,
		cliArgs:         cliArgs,
	}
}

// Load compiles configuration values from all sources according to hierarchical priorities.
func (c *ConfigLoader) Load(configFilePath string) (types.Configuration, error) {
	// 1. Start with hardcoded defaults
	conf := types.Configuration{
		MemtableSizeBytes:   DefaultMemtableSize,
		CompactionThreshold: DefaultCompactionThreshold,
		SyncWrites:          DefaultSyncWrites,
		Parallelism:         DefaultParallelism,
		MaxRetries:          DefaultMaxRetries,
		Timeout:             DefaultTimeout,
	}

	// 2. Override with Config File if present
	if configFilePath != "" {
		if err := c.loadFromFile(configFilePath, &conf); err != nil {
			return conf, errors.NewConfigurationError("failed to load config file", err)
		}
	}

	// 3. Override with Environment Variables
	c.loadFromEnv(&conf)

	// 4. Override with CLI Arguments
	c.loadFromCLI(&conf)

	// 5. Override with explicit program base dir override if set
	if c.baseDirOverride != "" {
		conf.BaseDir = c.baseDirOverride
	}

	// Validate resulting configuration
	if err := c.Validate(conf); err != nil {
		return conf, err
	}

	return conf, nil
}

func (c *ConfigLoader) loadFromFile(path string, conf *types.Configuration) error {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // config file optional
		}
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue // skip empty and comments
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])

		switch key {
		case "base_dir":
			conf.BaseDir = val
		case "memtable_size_bytes":
			v, err := strconv.ParseInt(val, 10, 64)
			if err != nil {
				return fmt.Errorf("memtable_size_bytes: %w", err)
			}
			conf.MemtableSizeBytes = v
		case "compaction_threshold":
			v, err := strconv.Atoi(val)
			if err != nil {
				return fmt.Errorf("compaction_threshold: %w", err)
			}
			conf.CompactionThreshold = v
		case "sync_writes":
			v, err := strconv.ParseBool(val)
			if err != nil {
				return fmt.Errorf("sync_writes: %w", err)
			}
			conf.SyncWrites = v
		case "parallelism":
			v, err := strconv.Atoi(val)
			if err != nil {
				return fmt.Errorf("parallelism: %w", err)
			}
			conf.Parallelism = v
		case "max_retries":
			v, err := strconv.Atoi(val)
			if err != nil {
				return fmt.Errorf("max_retries: %w", err)
			}
			conf.MaxRetries = v
		case "timeout_seconds":
			v, err := strconv.Atoi(val)
			if err != nil {
				return fmt.Errorf("timeout_seconds: %w", err)
			}
			conf.Timeout = time.Duration(v) * time.Second
		}
	}
	return scanner.Err()
}

func (c *ConfigLoader) loadFromEnv(conf *types.Configuration) {
	if val := os.Getenv("PEBBLEDB_ATF_BASE_DIR"); val != "" {
		conf.BaseDir = val
	}
	if val := os.Getenv("PEBBLEDB_ATF_MEMTABLE_SIZE"); val != "" {
		if v, err := strconv.ParseInt(val, 10, 64); err == nil {
			conf.MemtableSizeBytes = v
		}
	}
	if val := os.Getenv("PEBBLEDB_ATF_COMPACTION_THRESHOLD"); val != "" {
		if v, err := strconv.Atoi(val); err == nil {
			conf.CompactionThreshold = v
		}
	}
	if val := os.Getenv("PEBBLEDB_ATF_SYNC_WRITES"); val != "" {
		if v, err := strconv.ParseBool(val); err == nil {
			conf.SyncWrites = v
		}
	}
	if val := os.Getenv("PEBBLEDB_ATF_PARALLELISM"); val != "" {
		if v, err := strconv.Atoi(val); err == nil {
			conf.Parallelism = v
		}
	}
	if val := os.Getenv("PEBBLEDB_ATF_MAX_RETRIES"); val != "" {
		if v, err := strconv.Atoi(val); err == nil {
			conf.MaxRetries = v
		}
	}
}

func (c *ConfigLoader) loadFromCLI(conf *types.Configuration) {
	for _, arg := range c.cliArgs {
		if !strings.HasPrefix(arg, "--") {
			continue
		}
		parts := strings.SplitN(arg[2:], "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := parts[0]
		val := parts[1]

		switch key {
		case "base-dir":
			conf.BaseDir = val
		case "memtable-size":
			if v, err := strconv.ParseInt(val, 10, 64); err == nil {
				conf.MemtableSizeBytes = v
			}
		case "compaction-threshold":
			if v, err := strconv.Atoi(val); err == nil {
				conf.CompactionThreshold = v
			}
		case "sync-writes":
			if v, err := strconv.ParseBool(val); err == nil {
				conf.SyncWrites = v
			}
		case "parallelism":
			if v, err := strconv.Atoi(val); err == nil {
				conf.Parallelism = v
			}
		case "max-retries":
			if v, err := strconv.Atoi(val); err == nil {
				conf.MaxRetries = v
			}
		}
	}
}

// Validate checks that all required configuration fields are set within acceptable ranges.
func (c *ConfigLoader) Validate(conf types.Configuration) error {
	if conf.BaseDir == "" {
		return errors.NewConfigurationError("base directory must not be empty", nil)
	}
	if conf.MemtableSizeBytes < 256 {
		return errors.NewConfigurationError("memtable size must be at least 256 bytes", nil)
	}
	if conf.Parallelism < 1 {
		return errors.NewConfigurationError("parallelism must be at least 1", nil)
	}
	if conf.MaxRetries < 0 {
		return errors.NewConfigurationError("max retries must be non-negative", nil)
	}
	if conf.Timeout < 1*time.Second {
		return errors.NewConfigurationError("timeout must be at least 1 second", nil)
	}
	return nil
}
