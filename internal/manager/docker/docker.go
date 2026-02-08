package docker

import (
	"bytes"
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	"lognorth/internal/manager/config"
	"lognorth/internal/manager/logging"
)

const (
	NetworkName      = "lognorth-network"
	CaddyName        = "lognorth-caddy"
	AppName          = "lognorth-app"
	MaxRetries       = 3
	HealthCheckTries = 10
)

//go:embed templates/Caddyfile.tmpl
var caddyfileTemplate string

type Docker struct {
	logger *logging.Logger
}

func NewDocker(logger *logging.Logger) *Docker {
	return &Docker{logger: logger}
}

func (d *Docker) RunCommand(args ...string) (string, error) {
	d.logger.Debug("Running docker %s", strings.Join(args, " "))
	var stdout, stderr bytes.Buffer
	cmd := exec.Command("docker", args...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%w - %s", err, stderr.String())
	}
	return stdout.String(), nil
}

func (d *Docker) EnsureInstalled() error {
	if version, err := d.RunCommand("version"); err == nil {
		d.logger.Success("Docker is installed")
		d.logger.Debug("Version: %s", strings.TrimSpace(strings.Split(version, "\n")[0]))
		return nil
	}

	d.logger.Info("Docker not found, installing...")
	output, err := exec.Command("bash", "-c", "curl -fsSL https://get.docker.com | sh").CombinedOutput()
	if err != nil {
		d.logger.Error("Docker installation failed: %s", string(output))
		return fmt.Errorf("install failed: %w", err)
	}
	d.logger.Success("Docker installed successfully")

	for _, cmd := range [][]string{
		{"systemctl", "start", "docker"},
		{"systemctl", "enable", "docker"},
	} {
		if err := exec.Command(cmd[0], cmd[1:]...).Run(); err != nil {
			return fmt.Errorf("%s failed: %w", cmd[1], err)
		}
	}

	return nil
}

func (d *Docker) Deploy(conf *config.Config) error {
	data := conf.GetData()
	dataDir := data.InstallDir

	// Create directories with proper ownership (UID 10001 = lognorth user in container)
	appDirs := []string{
		filepath.Join(dataDir, "storage"),
		filepath.Join(dataDir, "logs"),
	}
	caddyDirs := []string{
		filepath.Join(dataDir, "caddy"),
		filepath.Join(dataDir, "caddy", "config"),
	}
	for _, dir := range append(appDirs, caddyDirs...) {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create dir %s: %w", dir, err)
		}
	}
	// Set ownership for app directories to lognorth user (UID 10001)
	for _, dir := range appDirs {
		if err := os.Chown(dir, 10001, 10001); err != nil {
			d.logger.Warn("Could not set ownership on %s: %v", dir, err)
		}
	}

	// Create network
	if _, err := d.RunCommand("network", "inspect", NetworkName); err != nil {
		d.logger.Info("Creating Docker network...")
		if _, err := d.RunCommand("network", "create", NetworkName); err != nil {
			return fmt.Errorf("create network: %w", err)
		}
	}

	// Generate Caddyfile
	caddyFile := filepath.Join(dataDir, "Caddyfile")
	caddyContent, err := d.generateCaddyfile(data)
	if err != nil {
		return fmt.Errorf("generate Caddyfile: %w", err)
	}
	if err := os.WriteFile(caddyFile, []byte(caddyContent), 0o644); err != nil {
		return fmt.Errorf("write Caddyfile: %w", err)
	}

	// Pull images
	for _, image := range []string{data.AppImage, data.CaddyImage} {
		d.logger.Info("Pulling %s...", image)
		for i := 0; i < MaxRetries; i++ {
			if _, err := d.RunCommand("pull", image); err == nil {
				break
			} else if i == MaxRetries-1 {
				return fmt.Errorf("pull %s failed: %w", image, err)
			}
			time.Sleep(time.Duration(i+1) * 2 * time.Second)
		}
	}

	// Deploy app
	d.logger.Info("Deploying LogNorth app...")
	if err := d.deployApp(data); err != nil {
		return fmt.Errorf("deploy app: %w", err)
	}

	// Wait for app health
	if err := d.waitForHealth(AppName); err != nil {
		d.logger.Error("App health check failed")
		d.showLogs(AppName)
		return err
	}

	// Deploy Caddy
	d.logger.Info("Deploying Caddy (SSL)...")
	if err := d.deployCaddy(data, caddyFile); err != nil {
		return fmt.Errorf("deploy caddy: %w", err)
	}

	return nil
}

func (d *Docker) deployApp(data config.ConfigData) error {
	d.stopAndRemove(AppName)

	_, err := d.RunCommand("run", "-d",
		"--name", AppName,
		"--network", NetworkName,
		"-v", filepath.Join(data.InstallDir, "storage")+":/app/storage",
		"-v", filepath.Join(data.InstallDir, "logs")+":/app/logs",
		"-e", "LOGNORTH_ENV=production",
		"-e", "LOGNORTH_PORT=8080",
		"-e", "LOGNORTH_DOMAIN="+data.Domain,
		"-e", "LOGNORTH_PRIVATE_KEY="+data.PrivateKey,
		"--memory=512m",
		"--restart", "unless-stopped",
		data.AppImage,
	)
	return err
}

func (d *Docker) deployCaddy(data config.ConfigData, caddyFile string) error {
	d.stopAndRemove(CaddyName)

	_, err := d.RunCommand("run", "-d",
		"--name", CaddyName,
		"--network", NetworkName,
		"-p", "80:80", "-p", "443:443", "-p", "443:443/udp",
		"-v", caddyFile+":/etc/caddy/Caddyfile:ro",
		"-v", filepath.Join(data.InstallDir, "caddy")+":/data",
		"-v", filepath.Join(data.InstallDir, "caddy", "config")+":/config",
		"-e", "DOMAIN="+data.Domain,
		"--memory=256m",
		"--restart", "unless-stopped",
		data.CaddyImage,
	)
	return err
}

func (d *Docker) stopAndRemove(name string) {
	d.RunCommand("stop", name)
	d.RunCommand("rm", "-f", name)
}

func (d *Docker) waitForHealth(name string) error {
	d.logger.Info("Waiting for %s to become healthy...", name)
	for i := 0; i < HealthCheckTries; i++ {
		if _, err := d.RunCommand("exec", name, "curl", "-sf", "http://localhost:8080/_health"); err == nil {
			d.logger.Success("%s is healthy", name)
			return nil
		}
		time.Sleep(3 * time.Second)
	}
	return fmt.Errorf("%s not healthy after %d attempts", name, HealthCheckTries)
}

func (d *Docker) showLogs(name string) {
	logs, _ := d.RunCommand("logs", "--tail", "50", name)
	if logs != "" {
		d.logger.Error("Container logs:\n%s", logs)
	}
}

func (d *Docker) generateCaddyfile(data config.ConfigData) (string, error) {
	tmpl, err := template.New("caddyfile").Parse(caddyfileTemplate)
	if err != nil {
		return "", fmt.Errorf("parse template: %w", err)
	}

	tplData := struct {
		Domain string
	}{
		Domain: data.Domain,
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, tplData); err != nil {
		return "", fmt.Errorf("execute template: %w", err)
	}

	return buf.String(), nil
}

func (d *Docker) IsRunning(name string) bool {
	out, err := d.RunCommand("ps", "-q", "-f", "name="+name)
	return err == nil && strings.TrimSpace(out) != ""
}

func (d *Docker) VerifyContainersRunning() (bool, error) {
	appRunning := d.IsRunning(AppName)
	caddyRunning := d.IsRunning(CaddyName)
	return appRunning && caddyRunning, nil
}
