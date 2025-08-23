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

func listTarballContent(tarballPath string) (content []string, err error) {
	file, err := os.Open(tarballPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	tr := tar.NewReader(file)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		switch header.Typeflag {
		case tar.TypeDir:
			continue
		default:
			content = append(content, header.Name)
		}
	}

	return content, nil
}

func readTarballFile(tarballPath, fileToExtract string) (*tarballFileReader, error) {
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

func extractTarballFile(tarballPath, fileToExtract string, workingDirectory string, uid, gid int) (err error) {
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

		// Skip if filename does not match
		if header.Name != fileToExtract {
			continue
		}

		// Trim directory name from header name
		header.Name = strings.Split(header.Name, "/")[len(strings.Split(header.Name, "/"))-1]
		outputPath := path.Join(workingDirectory, header.Name)

		switch header.Typeflag {
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
			if uid >= 0 && gid >= 0 {
				err = file.Chown(uid, gid)
				if err != nil {
					return err
				}
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

	return nil
}

func extractTarballDirectory(tarballPath, directoryToExtract, workingDirectory string, uid, gid int) (err error) {
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

				// Set directory owner
				if uid >= 0 && gid >= 0 {
					err = os.Chown(outputPath, uid, gid)
					if err != nil {
						return err
					}
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
				if uid >= 0 && gid >= 0 {
					err = file.Chown(uid, gid)
					if err != nil {
						return err
					}
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
