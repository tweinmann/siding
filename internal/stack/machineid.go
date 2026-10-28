package stack

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

// MachineID returns the stable id for dev.siding.machine: a one-way hash of the operating
// system's machine id, so the raw id never appears in a label. Without one it uses, and on
// first use writes, a random id in <home>/.siding/machine-id.
func MachineID(osID, home string) (string, error) {
	if osID = strings.TrimSpace(osID); osID != "" {
		sum := sha256.Sum256([]byte("siding-machine-id:" + osID))
		return hex.EncodeToString(sum[:])[:16], nil
	}
	path := filepath.Join(home, ".siding", "machine-id")
	if b, err := os.ReadFile(path); err == nil {
		if id := strings.TrimSpace(string(b)); id != "" {
			return id, nil
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return "", fmt.Errorf("read machine id: %w", err)
	}
	var raw [8]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	id := hex.EncodeToString(raw[:])
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", fmt.Errorf("write machine id: %w", err)
	}
	if err := os.WriteFile(path, []byte(id+"\n"), 0o644); err != nil {
		return "", fmt.Errorf("write machine id: %w", err)
	}
	return id, nil
}

var platformUUID = regexp.MustCompile(`"IOPlatformUUID" = "([^"]+)"`)

// OSMachineID returns the operating system's machine id: /etc/machine-id on Linux, the
// platform UUID on macOS, or "" when there is none.
func OSMachineID() string {
	if runtime.GOOS == "darwin" {
		out, err := exec.Command("ioreg", "-rd1", "-c", "IOPlatformExpertDevice").Output()
		if err != nil {
			return ""
		}
		if m := platformUUID.FindSubmatch(out); m != nil {
			return string(m[1])
		}
		return ""
	}
	b, err := os.ReadFile("/etc/machine-id")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}
