package installer

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"

	"lognorth/internal/manager/config"
	"lognorth/internal/manager/docker"
	"lognorth/internal/manager/logging"
	"lognorth/internal/manager/requirements"
)

const (
	DefaultInstallDir = "/opt/lognorth"
	DefaultBinaryPath = "/usr/local/bin/lognorth"
)

type Installer struct {
	logger     *logging.Logger
	config     *config.Config
	docker     *docker.Docker
	binaryPath string
}

func NewInstaller(logger *logging.Logger) *Installer {
	return &Installer{
		logger:     logger,
		config:     config.NewConfig(logger),
		docker:     docker.NewDocker(logger),
		binaryPath: DefaultBinaryPath,
	}
}

func (i *Installer) Docker() *docker.Docker {
	return i.docker
}

func (i *Installer) RunCompleteInstallation() error {
	totalSteps := 6

	// Step 1: Display welcome and collect config
	i.displayWelcome()
	reader := bufio.NewReader(os.Stdin)
	if err := i.config.CollectFromUser(reader); err != nil {
		return fmt.Errorf("configuration failed: %w", err)
	}

	// Step 2: Check requirements
	i.logger.Info("Step 1/%d: Checking system requirements", totalSteps)
	checker := requirements.NewChecker(i.logger)
	if err := checker.CheckSystemRequirements(); err != nil {
		return fmt.Errorf("requirements check failed: %w", err)
	}

	// Step 3: Install Docker
	i.logger.Info("Step 2/%d: Installing Docker", totalSteps)
	if err := i.docker.EnsureInstalled(); err != nil {
		return fmt.Errorf("Docker installation failed: %w", err)
	}

	// Step 4: Configure system
	i.logger.Info("Step 3/%d: Configuring system", totalSteps)
	if err := i.configureSystem(); err != nil {
		return fmt.Errorf("configuration failed: %w", err)
	}

	// Step 5: Deploy
	i.logger.Info("Step 4/%d: Deploying application", totalSteps)
	if err := i.docker.Deploy(i.config); err != nil {
		return fmt.Errorf("deployment failed: %w", err)
	}

	// Step 6: Install binary for updates
	i.logger.Info("Step 5/%d: Setting up updates", totalSteps)
	if err := i.installBinary(); err != nil {
		i.logger.Warn("Failed to install binary: %v", err)
	}

	// Step 7: Verify
	i.logger.Info("Step 6/%d: Verifying installation", totalSteps)
	if err := i.verify(); err != nil {
		return fmt.Errorf("verification failed: %w", err)
	}

	return nil
}

func (i *Installer) displayWelcome() {
	fmt.Println()
	fmt.Println("╔═══════════════════════════════════════╗")
	fmt.Println("║      LogNorth Installer               ║")
	fmt.Println("╚═══════════════════════════════════════╝")
	fmt.Println()
	fmt.Println("Requirements:")
	fmt.Println("  • Ports 80 and 443 available")
	fmt.Println("  • Root privileges")
	fmt.Println("  • Internet connection")
	fmt.Println()
	fmt.Println("SSL certificates provided by Let's Encrypt")
	fmt.Println()
}

func (i *Installer) configureSystem() error {
	data := i.config.GetData()

	// Create install directory
	if err := os.MkdirAll(data.InstallDir, 0o755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Save config
	envFile := filepath.Join(data.InstallDir, ".env")
	if _, err := os.Stat(envFile); os.IsNotExist(err) {
		if err := i.config.SaveToFile(envFile); err != nil {
			return fmt.Errorf("failed to save config: %w", err)
		}
	} else {
		// Load existing config to preserve private key
		oldConfig := config.NewConfig(i.logger)
		if err := oldConfig.LoadFromFile(envFile); err == nil {
			oldData := oldConfig.GetData()
			if oldData.PrivateKey != "" {
				newData := i.config.GetData()
				newData.PrivateKey = oldData.PrivateKey
				i.config.SetData(newData)
			}
		}
		if err := i.config.SaveToFile(envFile); err != nil {
			return fmt.Errorf("failed to save config: %w", err)
		}
	}

	if err := i.config.Validate(); err != nil {
		return err
	}

	i.logger.Success("Configuration saved to %s", envFile)
	return nil
}

func (i *Installer) installBinary() error {
	currentExe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}

	sourceData, err := os.ReadFile(currentExe)
	if err != nil {
		return fmt.Errorf("failed to read binary: %w", err)
	}

	if err := os.WriteFile(i.binaryPath, sourceData, 0755); err != nil {
		return fmt.Errorf("failed to install binary: %w", err)
	}

	i.logger.Success("Binary installed to %s", i.binaryPath)
	return nil
}

func (i *Installer) verify() error {
	running, err := i.docker.VerifyContainersRunning()
	if err != nil {
		return err
	}
	if !running {
		return fmt.Errorf("containers not running properly")
	}

	i.logger.Success("All containers running")
	return nil
}

func (i *Installer) DisplayCompletionMessage() {
	data := i.config.GetData()

	fmt.Println()
	fmt.Println("╔═══════════════════════════════════════╗")
	fmt.Println("║      Installation Complete!           ║")
	fmt.Println("╚═══════════════════════════════════════╝")
	fmt.Println()
	fmt.Printf("Dashboard: https://%s\n", data.Domain)
	fmt.Println()
	fmt.Println("Complete setup by creating your admin account at the URL above.")
	fmt.Println()
	fmt.Println("Useful commands:")
	fmt.Println("  lognorth update    - Update to latest version")
	fmt.Println("  lognorth status    - Check system status")
	fmt.Println()
}
