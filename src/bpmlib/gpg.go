package bpmlib

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"strings"
)

func InitializeKeyring(rootDir string) error {
	gpgHomedir := path.Join(rootDir, "/var/lib/bpm/gpg")

	// Create GPG directory
	err := os.Mkdir(gpgHomedir, 0700)
	if err != nil && !os.IsExist(err) {
		return err
	}

	// Get number of secret keys
	cmd := exec.Command("gpg", "--homedir="+gpgHomedir, "--list-secret-keys", "--with-colons")
	output, err := cmd.Output()
	if err != nil {
		return err
	}
	secretKeysLineCount := len(strings.Split(strings.TrimSpace(string(output)), "\n"))

	// Create signing key
	if secretKeysLineCount <= 1 {
		cmd := exec.Command("gpg", "--homedir="+gpgHomedir, "--batch", "--gen-key")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Stdin = strings.NewReader(`%echo Creating signing key...
Key-Type: RSA
Key-Length: 4096
Key-Usage: sign
Name-Real: BPM signing key
Name-Email: bpm@localhost
Expire-Date: 0
%no-protection
%commit
%echo Done`)

		err = cmd.Run()
		if err != nil {
			return err
		}
	}

	return nil
}

func IsKeyringInitialized(rootDir string) bool {
	gpgHomedir := path.Join(rootDir, "/var/lib/bpm/gpg")

	// Check if gpg directory exists
	if stat, err := os.Stat(gpgHomedir); err != nil || !stat.IsDir() {
		return false
	}

	// Get number of secret keys
	cmd := exec.Command("gpg", "--homedir="+gpgHomedir, "--list-secret-keys", "--with-colons")
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	secretKeysLineCount := len(strings.Split(strings.TrimSpace(string(output)), "\n"))

	// Return false if no signing key has been created
	if secretKeysLineCount <= 1 {
		return false
	}

	return true
}

func PopulateKeyring(rootDir string) error {
	gpgHomedir := path.Join(rootDir, "/var/lib/bpm/gpg")
	keyringsDir := path.Join(rootDir, "/var/lib/bpm/keyrings")

	dirEntries, err := os.ReadDir(keyringsDir)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	// Remove removed keys
	for _, entry := range dirEntries {
		if entry.IsDir() {
			continue
		}
		if !strings.HasSuffix(entry.Name(), ".revoked") {
			continue
		}

		data, err := os.ReadFile(path.Join(keyringsDir, entry.Name()))
		if err != nil {
			return err
		}

		// Loop over all key IDs
		for entry := range strings.SplitSeq(strings.TrimSpace(string(data)), "\n") {
			// Ensure key ID exists
			err := exec.Command("gpg", "--homedir="+gpgHomedir, "--list-keys", entry).Run()
			if err != nil {
				continue
			}

			cmd := exec.Command("gpg", "--homedir="+gpgHomedir, "--batch", "--yes", "--delete-secret-and-public-keys", entry)

			err = cmd.Run()
			if err != nil {
				return err
			}
		}
	}

	// Import all keyrings
	for _, entry := range dirEntries {
		if entry.IsDir() {
			continue
		}
		if !strings.HasSuffix(entry.Name(), ".pgp") && !strings.HasSuffix(entry.Name(), ".asc") {
			continue
		}

		cmd := exec.Command("gpg", "--homedir="+gpgHomedir, "--import", path.Join(keyringsDir, entry.Name()))

		err := cmd.Run()
		if err != nil {
			return err
		}
	}

	// Trust keys
	for _, entry := range dirEntries {
		if entry.IsDir() {
			continue
		}
		if !strings.HasSuffix(entry.Name(), ".trustdb") {
			continue
		}

		data, err := os.ReadFile(path.Join(keyringsDir, entry.Name()))
		if err != nil {
			return err
		}

		// Loop over all key IDs
		for entry := range strings.SplitSeq(strings.TrimSpace(string(data)), "\n") {
			keyID := strings.Split(entry, ":")[0]

			// Ensure key ID exists
			err := exec.Command("gpg", "--homedir="+gpgHomedir, "--list-keys", keyID).Run()
			if err != nil {
				continue
			}

			// Sign key
			cmd := exec.Command("gpg", "--homedir="+gpgHomedir, "--command-fd=0", "--batch", "--lsign-key", keyID)
			cmd.Stdin = strings.NewReader("y\ny\n")

			err = cmd.Run()
			if err != nil {
				return err
			}
		}

		// Import owner trust database
		cmd := exec.Command("gpg", "--homedir="+gpgHomedir, "--import-ownertrust", path.Join(keyringsDir, entry.Name()))

		err = cmd.Run()
		if err != nil {
			return err
		}
	}

	return nil
}

func VerifySignature(filename, signature string, requireTrusted bool, rootDir string) error {
	gpgHomedir := path.Join(rootDir, "/var/lib/bpm/gpg")

	if _, err := os.Stat(gpgHomedir); err != nil {
		return err
	}

	cmd := exec.Command("gpg", "--homedir="+gpgHomedir, "--status-fd=3", "--verify", signature, filename)
	pipeReader, pipeWriter, err := os.Pipe()
	if err != nil {
		return err
	}
	defer pipeReader.Close()
	defer pipeWriter.Close()
	cmd.ExtraFiles = append(cmd.ExtraFiles, pipeWriter)

	err = cmd.Run()
	if err != nil {
		return err
	}
	pipeWriter.Close()

	if requireTrusted {
		data, err := io.ReadAll(pipeReader)
		if err != nil {
			return err
		}

		dataStr := string(data)
		if !strings.Contains(dataStr, "[GNUPG:] TRUST_FULLY") && !strings.Contains(dataStr, "[GNUPG:] TRUST_ULTIMATE") {
			return fmt.Errorf("signature verified but not trusted")
		}
	}

	return err
}
