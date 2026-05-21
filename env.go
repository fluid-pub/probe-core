package core

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
)

// LoadEnvSecrets loads environment variables from an env.secrets file.
// It searches for the file in the following order:
// 1. In the executable's directory
// 2. In the current working directory
// 3. At the specified path (if provided)
//
// The function is non-fatal - if the file is not found, it logs a warning and returns nil.
// This allows agents to work without env.secrets if all variables are set via other means.
func LoadEnvSecrets(secretsPath ...string) error {
	var secretsFile string

	// If a path is provided, use it
	if len(secretsPath) > 0 && secretsPath[0] != "" {
		secretsFile = secretsPath[0]
	} else {
		// Try to find env.secrets relative to the executable
		execPath, err := os.Executable()
		if err == nil {
			execDir := filepath.Dir(execPath)
			secretsFile = filepath.Join(execDir, "env.secrets")
		}

		// If not found in executable directory, try current directory
		if secretsFile == "" || fileExists(secretsFile) == false {
			secretsFile = "env.secrets"
		}
	}

	// Check if file exists
	if !fileExists(secretsFile) {
		log.Printf("Warning: env.secrets file not found at %s, skipping environment variable loading", secretsFile)
		return nil
	}

	file, err := os.Open(secretsFile)
	if err != nil {
		return fmt.Errorf("error opening env.secrets: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	loadedCount := 0
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Parse KEY=VALUE or export KEY="value" format
		line = strings.TrimPrefix(line, "export ")
		line = strings.TrimSpace(line)
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])
			// Remove quotes if present
			value = strings.Trim(value, `"'`)
			if key != "" && value != "" {
				os.Setenv(key, value)
				loadedCount++
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading env.secrets: %w", err)
	}

	if loadedCount > 0 {
		log.Printf("Loaded %d environment variables from %s", loadedCount, secretsFile)
	}

	return nil
}

// fileExists checks if a file exists
func fileExists(filename string) bool {
	info, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false
	}
	return !info.IsDir()
}
