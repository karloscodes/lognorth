package requirements

import (
	"fmt"
	"net"
	"os"
	"os/exec"

	"lognorth/internal/manager/logging"
)

type Checker struct {
	logger *logging.Logger
}

func NewChecker(logger *logging.Logger) *Checker {
	return &Checker{logger: logger}
}

func (c *Checker) CheckSystemRequirements() error {
	// Check root privileges
	if os.Geteuid() != 0 {
		return fmt.Errorf("this installer requires root privileges")
	}
	c.logger.Success("Root privileges confirmed")

	// Check ports 80 and 443
	if err := c.checkPort(80); err != nil {
		return fmt.Errorf("port 80 is in use: %w", err)
	}
	if err := c.checkPort(443); err != nil {
		return fmt.Errorf("port 443 is in use: %w", err)
	}
	c.logger.Success("Ports 80 and 443 are available")

	// Check curl
	if _, err := exec.LookPath("curl"); err != nil {
		return fmt.Errorf("curl is required but not installed")
	}

	return nil
}

func (c *Checker) checkPort(port int) error {
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return err
	}
	ln.Close()
	return nil
}
