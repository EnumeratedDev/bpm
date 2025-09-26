package bpmlib

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"strings"
)

func retrieveUrlData(u string) ([]byte, error) {
	resp, err := http.Get(u)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Load data into byte buffer
	buffer, err := io.ReadAll(resp.Body)

	return buffer, nil
}

func downloadFile(u, filepath string, perm os.FileMode) error {
	if strings.HasSuffix(filepath, "/") {
		return fmt.Errorf("Filepath must not end in '/'")
	}

	data, err := retrieveUrlData(u)
	if err != nil {
		return err
	}

	// Create parent directories
	err = os.MkdirAll(path.Dir(filepath), 0755)
	if err != nil {
		return err
	}

	// Create file
	file, err := os.Create(filepath)
	if err != nil {
		return err
	}
	defer file.Close()

	// Copy data
	_, err = file.Write(data)
	if err != nil {
		return err
	}

	// Set file permissions
	err = file.Chmod(perm)
	if err != nil {
		return err
	}

	return nil
}
