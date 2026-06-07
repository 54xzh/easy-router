package config

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	ListenAddr string
	DBPath     string
	SecretKey  string
}

func Load() (Config, error) {
	if err := loadDotEnv(".env"); err != nil {
		return Config{}, err
	}

	cfg := Config{
		ListenAddr: getEnv("EASY_ROUTER_ADDR", "127.0.0.1:2778"),
		DBPath:     getEnv("EASY_ROUTER_DB", "./data/easy-router.db"),
		SecretKey:  os.Getenv("EASY_ROUTER_SECRET_KEY"),
	}
	if cfg.SecretKey == "" {
		return Config{}, errors.New("缺少 EASY_ROUTER_SECRET_KEY。请在 .env 中写 EASY_ROUTER_SECRET_KEY=change-me-with-a-long-random-string，或设置系统环境变量")
	}
	if len(cfg.SecretKey) < 16 {
		return Config{}, fmt.Errorf("EASY_ROUTER_SECRET_KEY 至少需要 16 个字符")
	}
	return cfg, nil
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func loadDotEnv(path string) error {
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("读取 %s 失败：%w", path, err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for line := 1; scanner.Scan(); line++ {
		raw := strings.TrimSpace(scanner.Text())
		if raw == "" || strings.HasPrefix(raw, "#") {
			continue
		}
		if strings.HasPrefix(raw, "export ") {
			raw = strings.TrimSpace(strings.TrimPrefix(raw, "export "))
		}

		key, rawValue, ok := strings.Cut(raw, "=")
		if !ok {
			return fmt.Errorf("%s 第 %d 行格式错误：需要 KEY=value", path, line)
		}
		key = strings.TrimSpace(key)
		if !isValidEnvKey(key) {
			return fmt.Errorf("%s 第 %d 行格式错误：变量名只能包含字母、数字和下划线，且不能以数字开头", path, line)
		}

		value, err := parseEnvValue(rawValue)
		if err != nil {
			return fmt.Errorf("%s 第 %d 行格式错误：%w", path, line, err)
		}
		if os.Getenv(key) != "" {
			continue
		}
		if err := os.Setenv(key, value); err != nil {
			return fmt.Errorf("设置环境变量 %s 失败：%w", key, err)
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("读取 %s 失败：%w", path, err)
	}
	return nil
}

func parseEnvValue(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", nil
	}

	quote := value[0]
	if quote == '"' || quote == '\'' {
		end := closingQuoteIndex(value, quote)
		if end == -1 {
			return "", errors.New("引号没有闭合")
		}
		rest := strings.TrimSpace(value[end+1:])
		if rest != "" && !strings.HasPrefix(rest, "#") {
			return "", errors.New("引号后只能跟注释")
		}
		if quote == '\'' {
			return value[1:end], nil
		}
		return strconv.Unquote(value[:end+1])
	}

	return stripInlineComment(value), nil
}

func closingQuoteIndex(value string, quote byte) int {
	escaped := false
	for i := 1; i < len(value); i++ {
		if quote == '"' && escaped {
			escaped = false
			continue
		}
		if quote == '"' && value[i] == '\\' {
			escaped = true
			continue
		}
		if value[i] == quote {
			return i
		}
	}
	return -1
}

func stripInlineComment(value string) string {
	for i := 0; i < len(value); i++ {
		if value[i] == '#' && (i == 0 || value[i-1] == ' ' || value[i-1] == '\t') {
			return strings.TrimSpace(value[:i])
		}
	}
	return strings.TrimSpace(value)
}

func isValidEnvKey(key string) bool {
	if key == "" {
		return false
	}
	for i := 0; i < len(key); i++ {
		ch := key[i]
		ok := ch == '_' || ch >= 'A' && ch <= 'Z' || ch >= 'a' && ch <= 'z' || i > 0 && ch >= '0' && ch <= '9'
		if !ok {
			return false
		}
	}
	return key[0] < '0' || key[0] > '9'
}
