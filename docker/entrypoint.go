package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

const (
	workDir         = "/opt/zoraxy/config"
	zeroTierCfgDir  = "/opt/zoraxy/config/zerotier"
	zeroTierLibPath = "/var/lib/zerotier-one"

	startupGrace    = time.Second
	shutdownTimeout = 8 * time.Second
)

type process struct {
	name string
	cmd  *exec.Cmd
	done chan struct{}
	err  error
}

func (p *process) exited() bool {
	select {
	case <-p.done:
		return true
	default:
		return false
	}
}

func getenv(key string, def string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return def
}

func envTrue(key string, def string) bool {
	return strings.EqualFold(strings.TrimSpace(getenv(key, def)), "true")
}

func newCmd(name string, args ...string) *exec.Cmd {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd
}

func run(name string, args ...string) error {
	if err := newCmd(name, args...).Run(); err != nil {
		return fmt.Errorf("Command failed: %q - %w", append([]string{name}, args...), err)
	}
	return nil
}

func start(name string, args ...string) (*process, error) {
	p := &process{name: name, cmd: newCmd(name, args...), done: make(chan struct{})}
	if err := p.cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start %s: %w", name, err)
	}
	go func() {
		p.err = p.cmd.Wait()
		close(p.done)
	}()

	select {
	case <-p.done:
		return nil, fmt.Errorf("%s exited early: %v", name, p.err)
	case <-time.After(startupGrace):
		return p, nil
	}
}

func terminate(procs ...*process) {
	for _, p := range procs {
		if p != nil && !p.exited() {
			log.Printf("Terminating %s...", p.name)
			_ = p.cmd.Process.Signal(syscall.SIGTERM)
		}
	}

	deadline := time.After(shutdownTimeout)
	for _, p := range procs {
		if p == nil {
			continue
		}
		select {
		case <-p.done:
		case <-deadline:
			// Deadline hit: kill this and any remaining processes.
			for _, q := range procs {
				if q != nil && !q.exited() {
					log.Printf("%s did not exit in time, killing...", q.name)
					_ = q.cmd.Process.Kill()
				}
			}
			for _, q := range procs {
				if q != nil {
					<-q.done
				}
			}
			return
		}
	}
}

func linkZeroTierDir() error {
	if err := os.MkdirAll(zeroTierCfgDir, 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(zeroTierLibPath), 0o755); err != nil {
		return err
	}

	fi, err := os.Lstat(zeroTierLibPath)
	switch {
	case errors.Is(err, os.ErrNotExist):
		// nothing to do
	case err != nil:
		return err
	case fi.Mode()&os.ModeSymlink != 0:
		if target, err := os.Readlink(zeroTierLibPath); err == nil && filepath.Clean(target) == zeroTierCfgDir {
			return nil
		}
		if err := os.Remove(zeroTierLibPath); err != nil {
			return err
		}
	default:
		if err := os.Remove(zeroTierLibPath); err != nil {
			return fmt.Errorf("%s exists and is not a symlink or empty directory: %w", zeroTierLibPath, err)
		}
	}
	return os.Symlink(zeroTierCfgDir, zeroTierLibPath)
}

func removeZeroTierLink() {
	fi, err := os.Lstat(zeroTierLibPath)
	if err != nil || fi.Mode()&os.ModeSymlink == 0 {
		return
	}
	if err := os.Remove(zeroTierLibPath); err != nil {
		log.Printf("Failed to remove ZeroTier symlink: %v", err)
	}
}

func startZeroTier() (*process, error) {
	log.Println("Starting ZeroTier...")
	if err := linkZeroTierDir(); err != nil {
		return nil, fmt.Errorf("failed to set up ZeroTier directory: %w", err)
	}
	return start("zerotier-one")
}

func startZoraxy() (*process, error) {
	log.Println("Starting Zoraxy...")
	args := []string{
		"-autorenew=" + getenv("AUTORENEW", "86400"),
		"-cfgupgrade=" + getenv("CFGUPGRADE", "true"),
		"-db=" + getenv("DB", "auto"),
		"-docker=" + getenv("DOCKER", "true"),
		"-earlyrenew=" + getenv("EARLYRENEW", "30"),
		"-enablelog=" + getenv("ENABLELOG", "true"),
		"-fastgeoip=" + getenv("FASTGEOIP", "false"),
		"-mdns=" + getenv("MDNS", "true"),
		"-mdnsname=" + getenv("MDNSNAME", ""),
		"-noauth=" + getenv("NOAUTH", "false"),
		"-plugin=" + getenv("PLUGIN", "/opt/zoraxy/plugin/"),
		"-port=:" + getenv("PORT", "8000"),
		"-sshlb=" + getenv("SSHLB", "false"),
		"-version=" + getenv("VERSION", "false"),
		"-webroot=" + getenv("WEBROOT", "./www"),
	}
	return start("zoraxy", args...)
}

func realMain() int {
	log.SetFlags(0)

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGTERM, syscall.SIGINT)

	log.Println("Updating CA certificates...")
	if err := run("update-ca-certificates"); err != nil {
		log.Println(err)
		return 1
	}

	if err := os.Chdir(workDir); err != nil {
		log.Printf("Failed to change directory to %s: %v", workDir, err)
		return 1
	}

	if envTrue("UPDATE_GEOIP", "false") {
		log.Println("Updating GeoIP data...")
		if err := run("zoraxy", "-update_geoip=true"); err != nil {
			log.Println(err)
			return 1
		}
	}

	var zerotier, zoraxy *process
	var err error

	defer removeZeroTierLink()

	if envTrue("ZEROTIER", "false") {
		if zerotier, err = startZeroTier(); err != nil {
			log.Println(err)
			return 1
		}
	}

	if zoraxy, err = startZoraxy(); err != nil {
		log.Println(err)
		terminate(zerotier) // don't leave ZeroTier orphaned
		return 1
	}

	// Block until a shutdown signal arrives or either child dies.
	var zeroTierDone <-chan struct{} // nil channel blocks forever if ZeroTier is disabled
	if zerotier != nil {
		zeroTierDone = zerotier.done
	}

	code := 0
	select {
	case s := <-sigs:
		log.Printf("Received %v. Cleaning up...", s)
	case <-zoraxy.done:
		log.Printf("Zoraxy exited unexpectedly: %v", zoraxy.err)
		code = 1
	case <-zeroTierDone:
		log.Printf("ZeroTier-One exited unexpectedly: %v", zerotier.err)
		code = 1
	}

	terminate(zoraxy, zerotier)
	return code
}

func main() {
	os.Exit(realMain())
}
