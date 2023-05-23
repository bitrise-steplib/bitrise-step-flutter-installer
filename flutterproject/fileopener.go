package flutterproject

import (
	"io"
	"os"
)

type FileOpener interface {
	OpenFile(pth string) (io.Reader, error)
}

type fileOpener struct {
}

func NewFileOpener() FileOpener {
	return fileOpener{}
}

func (o fileOpener) OpenFile(pth string) (io.Reader, error) {
	f, err := os.Open(pth)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		} else {
			return nil, nil
		}
	}
	return f, nil
}
