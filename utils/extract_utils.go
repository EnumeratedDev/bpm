package utils

import (
	"archive/tar"
	"errors"
	"io"
	"os"
)

func ReadTarballContent(tarballPath, fileToExtract string) ([]byte, error) {
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
		if header.Name == fileToExtract {
			if header.Typeflag != tar.TypeReg {
				return nil, errors.New("file to extract must be a regular file")
			}

			bytes, err := io.ReadAll(tr)
			if err != nil {
				return nil, err
			}
			return bytes, nil
		}
	}

	return nil, errors.New("could not file in tarball")
}
