package bpmlib

import (
	"archive/tar"
	"errors"
	"io"
	"os"
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
