package dashboard

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ResolveMetaEnvPath returns the .env file Save writes to. It prefers
// ARGUS_ENV_FILE, then an existing .env in the working or executable
// directory (matching loadEnvFile in main), else <cwd>/.env.
func ResolveMetaEnvPath() string {
	if p := os.Getenv("ARGUS_ENV_FILE"); p != "" {
		return p
	}
	var candidates []string
	if cwd, err := os.Getwd(); err == nil {
		candidates = append(candidates, filepath.Join(cwd, ".env"))
	}
	if exe, err := os.Executable(); err == nil {
		d := filepath.Dir(exe)
		candidates = append(candidates, filepath.Join(d, ".env"))
		for i := 0; i < 6; i++ {
			parent := filepath.Dir(d)
			if parent == d {
				break
			}
			candidates = append(candidates, filepath.Join(parent, ".env"))
			d = parent
		}
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	if len(candidates) > 0 {
		return candidates[0]
	}
	return ".env"
}

func redactSecret(s, secret string) string {
	if secret != "" {
		s = strings.ReplaceAll(s, secret, "[redacted]")
	}
	return s
}

// writeMetaEnvFile atomically updates META_* keys in path, preserving other
// lines. The file (and directory) are created with restrictive permissions
// because it contains an API key.
func writeMetaEnvFile(path, baseURL, apiKey, model string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create config directory %s: %w", filepath.Dir(path), err)
	}
	existing, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read existing .env file %s: %w", path, err)
	}
	updates := map[string]string{"META_BASE_URL": baseURL, "META_MODEL": model}
	if apiKey != "" {
		updates["META_API_KEY"] = apiKey
	}
	seen := map[string]bool{}
	var out []string
	for _, line := range strings.Split(string(existing), "\n") {
		key, _, ok := strings.Cut(line, "=")
		key = strings.TrimSpace(key)
		if ok {
			if v, isUpdate := updates[key]; isUpdate {
				out = append(out, key+"="+v)
				seen[key] = true
				continue
			}
		}
		out = append(out, line)
	}
	for len(out) > 0 && strings.TrimSpace(out[len(out)-1]) == "" {
		out = out[:len(out)-1]
	}
	for _, k := range []string{"META_BASE_URL", "META_API_KEY", "META_MODEL"} {
		if v, ok := updates[k]; ok && !seen[k] {
			out = append(out, k+"="+v)
		}
	}
	content := strings.Join(out, "\n") + "\n"

	tmp, err := os.CreateTemp(filepath.Dir(path), ".env.tmp-*")
	if err != nil {
		return fmt.Errorf("create temp .env file in %s: %w", filepath.Dir(path), err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return fmt.Errorf("set permissions on temp .env file: %w", err)
	}
	if _, err := tmp.WriteString(content); err != nil {
		tmp.Close()
		return fmt.Errorf("write temp .env file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("sync temp .env file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp .env file: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("rename temp .env file to %s: %w", path, err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return fmt.Errorf("set permissions on .env file %s: %w", path, err)
	}
	return nil
}

// readMetaEnvFile parses META_* values back from path, used to verify a save.
func readMetaEnvFile(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reload .env file %s: %w", path, err)
	}
	vals := map[string]string{}
	for _, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		if strings.HasPrefix(k, "META_") {
			vals[k] = strings.Trim(strings.TrimSpace(v), "\"'")
		}
	}
	return vals, nil
}
