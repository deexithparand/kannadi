package iac

import (
	"io"
	"os"
)

type Backend io.ReadCloser

func NewFileReader(path string) (Backend, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}

	return Backend(f), nil
}
