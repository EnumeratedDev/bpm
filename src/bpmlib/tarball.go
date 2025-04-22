package bpmlib

import (
	"archive/tar"
	"errors"
	"io"
	"os"
	"path"
	"strings"
)

type tarballFileReader struct {
	tarReader *tar.Reader
	file      *os.File
}

func readTarballContent(tarballPath, fileToExtract string) (*tarballFileReader, error) {
	file, err := os.Open(tarballPath)
	if err != nil {
		return nil, err
	}

	tr := tar.NewReader(file)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if header.Name == fileToExtract {
			if header.Typeflag != tar.TypeReg {
				return nil, errors.New("file to extract must be a regular file")
			}

			return &tarballFileReader{
				tarReader: tr,
				file:      file,
			}, nil
		}
	}

	return nil, errors.New("could not file in tarball")
}

func extractTarballDirectory(tarballPath, directoryToExtract, workingDirectory string) (err error) {
	file, err := os.Open(tarballPath)
	if err != nil {
		return err
	}
	defer file.Close()

	tr := tar.NewReader(file)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		if strings.HasPrefix(header.Name, directoryToExtract+"/") {
			// Skip directory to extract
			if strings.TrimRight(header.Name, "/") == workingDirectory {
				continue
			}

			// Trim directory name from header name
			header.Name = strings.TrimPrefix(header.Name, directoryToExtract+"/")
			outputPath := path.Join(workingDirectory, header.Name)

			switch header.Typeflag {
			case tar.TypeDir:
				// Create directory
				err := os.MkdirAll(outputPath, 0755)
				if err != nil {
					return err
				}
			case tar.TypeReg:
				// Create file and set permissions
				file, err = os.Create(outputPath)
				if err != nil {
					return err
				}
				err := file.Chmod(header.FileInfo().Mode())
				if err != nil {
					return err
				}

				// Copy data to file
				_, err = io.Copy(file, tr)
				if err != nil {
					return err
				}

				// Close file
				file.Close()
			default:
				continue
			}
		}
	}

	return nil
}
