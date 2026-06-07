package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadReadsDotEnv(t *testing.T) {
	tmp := t.TempDir()
	restoreWorkingDir(t, tmp)
	unsetEnv(t, "EASY_ROUTER_SECRET_KEY")
	unsetEnv(t, "EASY_ROUTER_ADDR")
	unsetEnv(t, "EASY_ROUTER_DB")

	err := os.WriteFile(filepath.Join(tmp, ".env"), []byte(`
EASY_ROUTER_SECRET_KEY=from-dotenv-secret
EASY_ROUTER_ADDR=0.0.0.0:9999
EASY_ROUTER_DB=./data/test.db
`), 0600)
	if err != nil {
		t.Fatal(err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.SecretKey != "from-dotenv-secret" {
		t.Fatalf("SecretKey = %q, want %q", cfg.SecretKey, "from-dotenv-secret")
	}
	if cfg.ListenAddr != "0.0.0.0:9999" {
		t.Fatalf("ListenAddr = %q, want %q", cfg.ListenAddr, "0.0.0.0:9999")
	}
	if cfg.DBPath != "./data/test.db" {
		t.Fatalf("DBPath = %q, want %q", cfg.DBPath, "./data/test.db")
	}
}

func TestLoadKeepsExistingEnv(t *testing.T) {
	tmp := t.TempDir()
	restoreWorkingDir(t, tmp)
	t.Setenv("EASY_ROUTER_SECRET_KEY", "from-system-secret")

	err := os.WriteFile(filepath.Join(tmp, ".env"), []byte(`
EASY_ROUTER_SECRET_KEY=from-dotenv-secret
`), 0600)
	if err != nil {
		t.Fatal(err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.SecretKey != "from-system-secret" {
		t.Fatalf("SecretKey = %q, want %q", cfg.SecretKey, "from-system-secret")
	}
}

func TestLoadDotEnvParsesQuotesAndComments(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, ".env")
	unsetEnv(t, "DOTENV_TEST_DOUBLE")
	unsetEnv(t, "DOTENV_TEST_SINGLE")
	unsetEnv(t, "DOTENV_TEST_INLINE")
	unsetEnv(t, "DOTENV_TEST_EXISTING")
	t.Setenv("DOTENV_TEST_EXISTING", "system-value")

	err := os.WriteFile(path, []byte(`
# comment
DOTENV_TEST_DOUBLE="quoted value"
DOTENV_TEST_SINGLE='single quoted value'
DOTENV_TEST_INLINE=plain value # inline comment
DOTENV_TEST_EXISTING=file-value
`), 0600)
	if err != nil {
		t.Fatal(err)
	}

	if err := loadDotEnv(path); err != nil {
		t.Fatal(err)
	}
	if got := os.Getenv("DOTENV_TEST_DOUBLE"); got != "quoted value" {
		t.Fatalf("DOTENV_TEST_DOUBLE = %q, want %q", got, "quoted value")
	}
	if got := os.Getenv("DOTENV_TEST_SINGLE"); got != "single quoted value" {
		t.Fatalf("DOTENV_TEST_SINGLE = %q, want %q", got, "single quoted value")
	}
	if got := os.Getenv("DOTENV_TEST_INLINE"); got != "plain value" {
		t.Fatalf("DOTENV_TEST_INLINE = %q, want %q", got, "plain value")
	}
	if got := os.Getenv("DOTENV_TEST_EXISTING"); got != "system-value" {
		t.Fatalf("DOTENV_TEST_EXISTING = %q, want %q", got, "system-value")
	}
}

func restoreWorkingDir(t *testing.T, next string) {
	t.Helper()
	prev, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(next); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(prev)
	})
}

func unsetEnv(t *testing.T, key string) {
	t.Helper()
	prev, had := os.LookupEnv(key)
	if err := os.Unsetenv(key); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if had {
			_ = os.Setenv(key, prev)
			return
		}
		_ = os.Unsetenv(key)
	})
}
