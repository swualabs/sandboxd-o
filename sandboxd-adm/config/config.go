package config

import (
	"bufio"
	"os"
	"strconv"
	"strings"

	"sandboxd-o/pkg/envutil"
)

const DefaultStoreTable = "sbxadm_store"

type Config struct {
	StoreDynamoDBTable string
	OrchServer         string
	AWSProfile         string
	AWSRegion          string
}

func DefaultConfig() Config {
	return Config{
		StoreDynamoDBTable: DefaultStoreTable,
	}
}

func Load(path string) (Config, error) {
	if strings.TrimSpace(path) != "" {
		if err := loadEnvFile(path); err != nil {
			return Config{}, err
		}
	}

	cfg := DefaultConfig()
	cfg.StoreDynamoDBTable = envutil.Get("SBXADM_STORE_DYNAMODB", cfg.StoreDynamoDBTable)
	cfg.OrchServer = envutil.Get("SBXADM_ORCH_SERVER", cfg.OrchServer)
	cfg.AWSProfile = envutil.Get("AWS_PROFILE", "default")
	cfg.AWSRegion = envutil.Get("AWS_REGION", envutil.Get("AWS_DEFAULT_REGION", ""))

	return cfg, nil
}

// loadEnvFile never overwrites a variable already set in the process
// environment, so real env vars take precedence over the file.
func loadEnvFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")

		before, after, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}

		key := strings.TrimSpace(before)
		val := strings.TrimSpace(after)
		val = unquote(val)

		if key == "" {
			continue
		}
		if _, exists := os.LookupEnv(key); exists {
			continue
		}
		_ = os.Setenv(key, val)
	}

	return scanner.Err()
}

func unquote(v string) string {
	if len(v) >= 2 {
		if (v[0] == '"' && v[len(v)-1] == '"') || (v[0] == '\'' && v[len(v)-1] == '\'') {
			if unq, err := strconv.Unquote(v); err == nil {
				return unq
			}
			return v[1 : len(v)-1]
		}
	}
	return v
}
