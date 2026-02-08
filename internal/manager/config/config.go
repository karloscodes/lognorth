package config

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"strings"

	"lognorth/internal/manager/logging"
)

const (
	DefaultInstallDir = "/opt/lognorth"
	DefaultAppImage   = "karloscodes/lognorth:latest"
	DefaultCaddyImage = "caddy:2-alpine"
)

type ConfigData struct {
	InstallDir string
	Domain     string
	PrivateKey string
	AppImage   string
	CaddyImage string
}

type Config struct {
	logger *logging.Logger
	data   ConfigData
}

func NewConfig(logger *logging.Logger) *Config {
	return &Config{
		logger: logger,
		data: ConfigData{
			InstallDir: DefaultInstallDir,
			AppImage:   DefaultAppImage,
			CaddyImage: DefaultCaddyImage,
		},
	}
}

func (c *Config) GetData() ConfigData {
	return c.data
}

func (c *Config) SetData(data ConfigData) {
	c.data = data
}

func (c *Config) CollectFromUser(reader *bufio.Reader) error {
	// Get domain
	fmt.Print("\nEnter your domain (e.g., logs.example.com): ")
	domain, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("failed to read domain: %w", err)
	}
	c.data.Domain = strings.TrimSpace(domain)

	if c.data.Domain == "" {
		return fmt.Errorf("domain is required")
	}

	// Validate domain format
	if err := c.validateDomain(c.data.Domain); err != nil {
		return err
	}

	// Generate private key
	c.data.PrivateKey = generatePrivateKey()

	return nil
}

func (c *Config) validateDomain(domain string) error {
	// Basic validation
	if strings.Contains(domain, " ") {
		return fmt.Errorf("domain cannot contain spaces")
	}
	if strings.HasPrefix(domain, "http://") || strings.HasPrefix(domain, "https://") {
		return fmt.Errorf("domain should not include protocol (http:// or https://)")
	}

	// Check DNS resolution (warning only)
	_, err := net.LookupHost(domain)
	if err != nil {
		c.logger.Warn("DNS lookup failed for %s - make sure DNS is configured", domain)
	}

	return nil
}

func (c *Config) Validate() error {
	if c.data.Domain == "" {
		return fmt.Errorf("domain is required")
	}
	if c.data.PrivateKey == "" {
		return fmt.Errorf("private key is required")
	}
	return nil
}

func (c *Config) SaveToFile(path string) error {
	content := fmt.Sprintf(`# LogNorth Configuration
LOGNORTH_DOMAIN=%s
LOGNORTH_PRIVATE_KEY=%s
LOGNORTH_ENV=production
`, c.data.Domain, c.data.PrivateKey)

	return os.WriteFile(path, []byte(content), 0600)
}

func (c *Config) LoadFromFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		switch key {
		case "LOGNORTH_DOMAIN":
			c.data.Domain = value
		case "LOGNORTH_PRIVATE_KEY":
			c.data.PrivateKey = value
		}
	}

	return nil
}

func generatePrivateKey() string {
	bytes := make([]byte, 32)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}
