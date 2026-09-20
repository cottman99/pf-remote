package shellbinding

import (
	"bufio"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/cottman99/pf-remote/pkg/contracts"
)

const maxPublicHostKeyFile = 32 << 10

// DiscoverSystemHostKeys reads only OpenSSH public host-key files. It never
// reads private keys, executes ssh-keyscan, probes a network endpoint, or
// changes the SSH service. Missing files are normal on machines without an
// OpenSSH server.
func DiscoverSystemHostKeys() ([]contracts.SSHHostKey, error) {
	return DiscoverHostKeys(defaultPublicHostKeyPaths()...)
}

// DiscoverHostKeys is the deterministic, testable form of system discovery.
func DiscoverHostKeys(paths ...string) ([]contracts.SSHHostKey, error) {
	keys := make([]contracts.SSHHostKey, 0, len(paths))
	for _, path := range paths {
		key, found, err := readPublicHostKey(path)
		if err != nil {
			return nil, err
		}
		if found {
			keys = append(keys, key)
		}
	}
	if len(keys) == 0 {
		return nil, nil
	}
	canonical, err := canonicalHostKeys(keys)
	if err != nil {
		return nil, errors.New("OpenSSH public host keys are invalid")
	}
	return canonical, nil
}

func defaultPublicHostKeyPaths() []string {
	if runtime.GOOS == "windows" {
		root := os.Getenv("ProgramData")
		if root == "" {
			return nil
		}
		return publicHostKeyPaths(filepath.Join(root, "ssh"))
	}
	return publicHostKeyPaths("/etc/ssh")
}

func publicHostKeyPaths(root string) []string {
	return []string{
		filepath.Join(root, "ssh_host_ed25519_key.pub"),
		filepath.Join(root, "ssh_host_ecdsa_key.pub"),
		filepath.Join(root, "ssh_host_rsa_key.pub"),
	}
}

func readPublicHostKey(path string) (contracts.SSHHostKey, bool, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return contracts.SSHHostKey{}, false, nil
	}
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Size() > maxPublicHostKeyFile {
		return contracts.SSHHostKey{}, false, errors.New("OpenSSH public host-key source is unsafe")
	}
	file, err := os.Open(path)
	if err != nil {
		return contracts.SSHHostKey{}, false, errors.New("OpenSSH public host key is unavailable")
	}
	defer file.Close()
	reader := bufio.NewReader(io.LimitReader(file, maxPublicHostKeyFile+1))
	content, err := io.ReadAll(reader)
	if err != nil || len(content) > maxPublicHostKeyFile {
		return contracts.SSHHostKey{}, false, errors.New("OpenSSH public host key could not be read safely")
	}
	fields := strings.Fields(string(content))
	if len(fields) < 2 {
		return contracts.SSHHostKey{}, false, errors.New("OpenSSH public host key is invalid")
	}
	key := contracts.SSHHostKey{Algorithm: fields[0], PublicKey: fields[1]}
	if _, err := Fingerprint(key); err != nil {
		return contracts.SSHHostKey{}, false, errors.New("OpenSSH public host key is invalid")
	}
	return key, true, nil
}
