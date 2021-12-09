package image

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"lxc-launcher/log"
	"os"
	"path/filepath"
	"strings"
)

type TgzPacker struct {
}

func NewTgzPacker() *TgzPacker {
	return &TgzPacker{}
}

func RemoveFile(filePath string) error {
	if FileExists(filePath) {
		err := os.RemoveAll(filePath)
		return err
	}
	return nil
}

func FileExists(path string) bool {
	_, err := os.Stat(path)
	if err != nil {
		if os.IsExist(err) {
			return true
		}
		return false
	}
	return true
}

func (tp *TgzPacker) dirExists(dir string) bool {
	info, err := os.Stat(dir)
	return (err == nil || os.IsExist(err)) && info.IsDir()
}

func (tp *TgzPacker) TarGz(sourceFullPath string, tarFileName string) (err error) {
	sourceInfo, err := os.Stat(sourceFullPath)
	if err != nil {
		return err
	}
	if err = RemoveFile(tarFileName); err != nil {
		return err
	}
	file := &os.File{}
	if FileExists(tarFileName) {
		file, err = os.OpenFile(tarFileName, os.O_WRONLY|os.O_APPEND, 0666)
		if err != nil {
			return err
		}
	} else {
		file, err = os.Create(tarFileName)
		if err != nil {
			return err
		}
	}

	defer func() {
		if err2 := file.Close(); err2 != nil && err == nil {
			err = err2
		}
	}()
	gWriter := gzip.NewWriter(file)
	defer func() {
		if err2 := gWriter.Close(); err2 != nil && err == nil {
			err = err2
		}
	}()
	tarWriter := tar.NewWriter(gWriter)
	defer func() {
		if err2 := tarWriter.Close(); err2 != nil && err == nil {
			err = err2
		}
	}()

	if sourceInfo.IsDir() {
		return tp.tarFolder(sourceFullPath, filepath.Base(sourceFullPath), tarWriter)
	}
	return tp.tarFile(sourceFullPath, tarWriter)
}

func (tp *TgzPacker) tarFile(sourceFullFile string, writer *tar.Writer) error {
	info, err := os.Stat(sourceFullFile)
	if err != nil {
		return err
	}

	header, err := tar.FileInfoHeader(info, "")
	if err != nil {
		return err
	}

	err = writer.WriteHeader(header)
	if err != nil {
		return err
	}

	fr, err := os.Open(sourceFullFile)
	if err != nil {
		return err
	}
	defer func() {

		if err2 := fr.Close(); err2 != nil && err == nil {
			err = err2
		}
	}()
	if _, err = io.Copy(writer, fr); err != nil {
		return err
	}
	return nil
}

func (tp *TgzPacker) tarFolder(sourceFullPath string, baseName string, writer *tar.Writer) error {
	baseFullPath := sourceFullPath
	return filepath.Walk(sourceFullPath, func(fileName string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		if fileName == baseFullPath {
			header.Name = baseName
		} else {
			header.Name = filepath.Join(baseName, strings.TrimPrefix(fileName, baseFullPath))
		}

		if err = writer.WriteHeader(header); err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		fr, err := os.Open(fileName)
		if err != nil {
			return err
		}
		defer fr.Close()
		if _, err := io.Copy(writer, fr); err != nil {
			return err
		}
		return nil
	})
}

func GetFileList(dir string) []string {
	fileList := make([]string, 0)
	err := filepath.Walk(dir,
		func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			fileList = append(fileList, path)
			return nil
		})
	if err != nil {
		log.Logger.Error(fmt.Sprintln("err: ", err))
	}
	return fileList
}
