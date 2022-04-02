package image

import (
	"fmt"
	cli "github.com/lxc/lxd/client"
	"github.com/lxc/lxd/shared/api"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	VM             = "virtual-machine"
	CONTAINER      = "container"
	CONTAINER_TYPE = "rootfs.squashfs"
	VM_TYPE        = "disk.qcow2"
	LXD_TYPE       = "lxd.tar.xz"
	COMPRESS_TYPE  = "gzip"
)

func (p *Puller) loadLXDImages() error {
	p.logger.Info(fmt.Sprintln("import image start...."))
	imagePathList := strings.Split(p.imageName, "/")
	imageAliaName := imagePathList[len(imagePathList)-1]
	// import images
	imImageErr := p.ImportLxdImages(imageAliaName)
	if imImageErr != nil {
		p.logger.Error(fmt.Sprintln("imImageErr: ", imImageErr))
		return imImageErr
	}
	return nil
}

func (p *Puller) ImportLxdImages(imageAliaName string) error {
	imageApi := api.ImagesPost{}
	imageType := api.InstanceType(VM)
	fileType := VM_TYPE
	if strings.Contains(p.imageName, CONTAINER) {
		imageType = api.InstanceType(CONTAINER)
		fileType = CONTAINER_TYPE
	}
	imageArgs := cli.ImageCreateArgs{Type: string(imageType)}
	for _, fileName := range p.FileNameList {
		baseName := filepath.Base(fileName)
		if strings.Contains(baseName, fileType) {
			fr, readErr := os.Open(fileName)
			if readErr != nil {
				p.logger.Info(fmt.Sprintf("%s, readErr: %s", fileType, readErr))
				return readErr
			}
			imageArgs.RootfsFile = fr
			imageArgs.RootfsName = fileName
		}

		if strings.Contains(baseName, LXD_TYPE) {
			fr, readErr := os.Open(fileName)
			if readErr != nil {
				p.logger.Info(fmt.Sprintf("%s, readErr: %s", LXD_TYPE, readErr))
				return readErr
			}
			imageArgs.MetaFile = fr
			imageArgs.MetaName = fileName
		}
	}
	imageApi.Filename = imageAliaName
	imageApi.ImagePut.Public = true
	imageApi.CompressionAlgorithm = COMPRESS_TYPE
	p.logger.Info(fmt.Sprintln("imageApi: ", imageApi, "\n imageArgs: ", imageArgs))
	p.logger.Info(fmt.Sprintf("start to create images,imageAliaName: %s", imageAliaName))
	op, creteImageErr := p.lxdClient.CreateImage(imageApi, imageArgs)
	if creteImageErr != nil {
		p.logger.Error(fmt.Sprintln("creteImageErr: ", creteImageErr))
		return creteImageErr
	}
	p.logger.Info(fmt.Sprintln("The image is imported successfully, ", op))
	imAliasErr := p.ImportLxdImageAlias(op, string(imageType), imageAliaName)
	if imAliasErr != nil {
		p.logger.Error(fmt.Sprintln("imAliasErr: ", imAliasErr))
		return imAliasErr
	}
	return nil
}

func (p *Puller) ImportLxdImageAlias(op cli.Operation, imageType, imageAliaName string) error {
	// Create image alias
	alias := api.ImageAliasesPost{}
	alias.Type = imageType
	alias.Name = imageAliaName
	alias.Description = imageAliaName
	opValue := op.Get()
	p.logger.Info(fmt.Sprintln("opValue.Resources: ", opValue.Resources, ",opValue.Metadata: ", opValue.Metadata,
		",opValue.Status:", opValue.Status, ",opValue.StatusCode: ", opValue.StatusCode,
		",opValue.Location: ", opValue.Location, ",opValue.Description： ", opValue.Description, ",opValue.ID: ", opValue.ID))
	for {
		getOp, _, opErr := p.lxdClient.GetOperation(opValue.ID)
		if opErr != nil {
			return opErr
		}
		if len(getOp.Metadata) > 0 {
			if fingerPrint, ok := getOp.Metadata["fingerprint"]; ok {
				alias.Target = fingerPrint.(string)
				delAliasErr := p.DeleteImageAlias(imageAliaName)
				if delAliasErr != nil {
					p.logger.Error(fmt.Sprintln("delAliasErr:", delAliasErr))
					return delAliasErr
				}
				aliasErr := p.lxdClient.CreateImageAlias(alias)
				if aliasErr != nil {
					p.logger.Error(fmt.Sprintln("aliasErr:", aliasErr))
					return aliasErr
				} else {
					p.logger.Info(fmt.Sprintln("Create image alias successfully, ", alias, ",getOp.ID:", getOp.ID))
					break
				}
			} else {
				time.Sleep(1 * time.Second)
			}
		} else {
			time.Sleep(1 * time.Second)
		}
	}
	return nil
}

func (p *Puller) DeleteImageAlias(imageAliaName string) error {
	imageExists, _ := p.lxdClient.CheckManageImageByAlias(imageAliaName)
	if imageExists {
		delImageErr := p.lxdClient.DeleteImageAlias(imageAliaName)
		if delImageErr != nil {
			fmt.Println("delImageErr: ", delImageErr)
			return delImageErr
		} else {
			p.logger.Info(fmt.Sprintln("Delete image alias successfully, imageAliaName: ", imageAliaName))
		}
	}
	return nil
}

func (p *Puller) DeleteInvalidImages() {
	images, getErr := p.lxdClient.GetImages()
	if getErr != nil {
		p.logger.Error(fmt.Sprintf("getErr: %v", getErr))
		return
	}
	if len(images) > 0 {
		for _, image := range images {
			if len(image.Aliases) == 0 && len(image.Fingerprint) > 0 {
				op, opErr := p.lxdClient.DeleteImage(image.Fingerprint)
				if opErr != nil {
					p.logger.Error(fmt.Sprintf("p.lxdClient.DeleteImage, Failed to delete mirror, opErr: %v", opErr))
				} else {
					p.logger.Info(fmt.Sprintln("Mirror deleted successfully, op: ", op))
				}
			}
		}
	}
}
