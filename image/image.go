package image

import (
	"errors"
	"fmt"
	cli "github.com/lxc/lxd/client"
	"github.com/lxc/lxd/shared/api"
	"lxc-launcher/log"
	"os"
	"strings"
)

const (
	VM        = "virtual-machine"
	CONTAINER = "container"
)

type FileInfo struct {
	FileName string
}

func (p *Puller) loadLXDImages() error {
	imageExists, _ := p.CheckImageExists()
	if imageExists {
		delImageErr := p.lxdClient.DeleteImageAlias(p.imageName)
		if delImageErr != nil {
			return delImageErr
		}
	}
	log.Logger.Info(fmt.Sprintf("start to create images %s", p.imageName))
	imageApi := api.ImagesPost{}
	imageAlias := api.ImageAlias{Name: p.imageName, Description: p.imageName}
	imageApi.Aliases = append(imageApi.Aliases, imageAlias)
	imageType := api.InstanceType(VM)
	if strings.Contains(p.imageName, CONTAINER) {
		imageType = api.InstanceType(CONTAINER)
	}
	imageArgs := cli.ImageCreateArgs{
		Type: string(imageType)}
	for _, fileName := range p.FileNameList {
		if strings.Contains(fileName, "rootfs.squashfs") {
			fileInfo := FileInfo{FileName: fileName}
			imageArgs.RootfsFile = fileInfo
			fileData := make([]byte, 0)
			imageArgs.RootfsFile.Read(fileData)
			imageArgs.RootfsName = fileName
		}
		if strings.Contains(fileName, "disk.qcow2") {
			fileInfo := FileInfo{FileName: fileName}
			imageArgs.MetaFile = fileInfo
			fileData := make([]byte, 0)
			imageArgs.MetaFile.Read(fileData)
			imageArgs.MetaName = fileName
		}
		if strings.Contains(fileName, "lxd.tar.xz") {
			imageApi.Filename = fileName
		}
	}
	op, creteImageErr := p.lxdClient.CreateImage(imageApi, imageArgs)
	if creteImageErr != nil {
		return creteImageErr
	}
	log.Logger.Info(fmt.Sprintf("The image is imported successfully, %v", op))
	return nil
}

func (read FileInfo) Read(data []byte) (int, error) {
	fp, fpErr := os.Open(read.FileName)
	if fpErr != nil {
		log.Logger.Error(fmt.Sprintf("fail to open the file, fileName: %s, err: %s", read.FileName, fpErr))
		return 0, fpErr
	}
	num, readErr := fp.Read(data)
	return num, readErr
}

func (p *Puller) DeleteInvalidImages() {
	images, getErr := p.lxdClient.GetImages()
	if getErr != nil {
		log.Logger.Error(fmt.Sprintf("getErr: %v", getErr))
		return
	}
	for _, image := range images {
		if len(image.Aliases) == 0 {
			op, opErr := p.lxdClient.DeleteImage(image.Fingerprint)
			if opErr != nil {
				log.Logger.Error(fmt.Sprintf("p.lxdClient.DeleteImage, Failed to delete mirror, opErr: %v", opErr))
			} else {
				log.Logger.Info(fmt.Sprintln("Mirror deleted successfully, op: ", op))
			}
		}
	}
}

func (p *Puller) CheckImageExists() (bool, error) {
	imageExists, err := p.lxdClient.CheckImageByAlias(p.imageName)
	if err != nil {
		return false, err
	}
	if !imageExists {
		return false, errors.New(fmt.Sprintf("unable to find image by alias %s", p.imageName))
	}
	return true, nil
}
