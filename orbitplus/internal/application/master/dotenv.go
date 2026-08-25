package master

import (
	"bufio"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strconv"
	"strings"
)

// loadDotEnv loads local key/value settings without overriding process variables.
func loadDotEnv(path string) error {
	file, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 1024), 1024*1024)
	for lineNumber := 1; scanner.Scan(); lineNumber++ {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		key, value, found := strings.Cut(line, "=")
		key = strings.TrimSpace(key)
		if !found || !validDotEnvKey(key) {
			return fmt.Errorf("invalid .env entry at line %d", lineNumber)
		}
		value, err = dotEnvValue(strings.TrimSpace(value))
		if err != nil {
			return fmt.Errorf("invalid .env value at line %d", lineNumber)
		}
		if _, exists := os.LookupEnv(key); !exists {
			if err := os.Setenv(key, value); err != nil {
				return err
			}
		}
	}
	return scanner.Err()
}

func validDotEnvKey(value string) bool {
	for index, character := range value {
		if !(character == '_' || character >= 'A' && character <= 'Z' || index > 0 && character >= '0' && character <= '9') {
			return false
		}
	}
	return value != ""
}

func dotEnvValue(value string) (string, error) {
	if len(value) < 2 || value[0] != value[len(value)-1] {
		return value, nil
	}
	if value[0] == '\'' {
		return value[1 : len(value)-1], nil
	}
	if value[0] == '"' {
		return strconv.Unquote(value)
	}
	return value, nil
}
