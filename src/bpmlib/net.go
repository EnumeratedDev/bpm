package bpmlib

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"strings"
)

func downloadFile(barText, u, filepath string, perm os.FileMode) error {
	if strings.HasSuffix(filepath, "/") {
		return fmt.Errorf("Filepath must not end in '/'")
	}

	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

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

	// Create progress bar
	bar := createProgressBar(resp.ContentLength, barText, false)
	defer bar.Close()

	// Copy data
	_, err = io.Copy(io.MultiWriter(file, bar), resp.Body)
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
