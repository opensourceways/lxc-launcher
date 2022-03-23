package image

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"go.uber.org/zap"
	"lxc-launcher/log"
	"lxc-launcher/lxd"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	SWR    = "swr"
	DOCKER = "docker"
)

//var loadLock sync.Mutex

type Handler struct {
	baseFolder   string
	metaEndpoint string
	worker       int64
	syncInterval int64
	imageCh      chan ImageDetail
	closeCh      chan bool
	logger       *zap.Logger
	user         string
	password     string
	lxdClient    *lxd.Client
}

type LXDImageResponse struct {
	Images []ImageDetail `json:"images"`
}

type LXDImageList struct {
	Images []string `json:"images"`
}

type ImageDetail struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

func NewImageHandler(username, password, baseFolder, metaEndpoint string, worker int64,
	syncInterval int64, lxdClient *lxd.Client, logger *zap.Logger) (*Handler, error) {

	return &Handler{
		user:         username,
		password:     password,
		baseFolder:   baseFolder,
		metaEndpoint: metaEndpoint,
		worker:       worker,
		syncInterval: syncInterval,
		imageCh:      make(chan ImageDetail, worker*32),
		closeCh:      make(chan bool, 1),
		lxdClient:    lxdClient,
		logger:       logger,
	}, nil
}

func (h *Handler) StartLoop() {
	// 1. Create a mirror of the instance
	err := h.pushImageLoadTask()
	if err != nil {
		h.logger.Warn(fmt.Sprintf("unable to list image details %s", err))
		return
	}
	for i := 1; i <= int(h.worker); i++ {
		h.logger.Info(fmt.Sprintf("starting to initialzie worker %d to load image.", i))
		go h.pullingImage(i, h.closeCh)
	}
	ticker := time.NewTicker(time.Duration(h.syncInterval) * time.Second)
	for {
		select {
		case <-ticker.C:
			err := h.pushImageLoadTask()
			if err != nil {
				h.logger.Warn(fmt.Sprintf("unable to list image details %s", err))
			}
			// Perform a delete operation on a stopped instance
			delErr := h.lxdClient.DeleteStopInstances("")
			if delErr != nil {
				log.Logger.Error(fmt.Sprintf("delErr: %s", delErr))
			}
		case _, ok := <-h.closeCh:
			if !ok {
				h.logger.Info("image handler received close event, quiting..")
				time.Sleep(5 * time.Second)
				return
			}
		}
	}
}

func (h *Handler) FakeLoop() {
	ticker := time.NewTicker(time.Duration(h.syncInterval) * time.Second)
	for {
		select {
		case <-ticker.C:
			h.logger.Info(fmt.Sprintf("perform fake sync triggered at %s", time.Now()))
		case _, ok := <-h.closeCh:
			if !ok {
				h.logger.Info("image handler received close event, quiting..")
				return
			}
		}
	}
}

func (h *Handler) Close() {
	close(h.closeCh)
}

func (h *Handler) GetImagePuller(detail ImageDetail) (*Puller, error) {
	if strings.ToLower(detail.Type) == DOCKER {
		return NewDockerIOV2ImagePuller(h.user, h.password, h.baseFolder, detail.Name, h.logger, h.lxdClient)
	} else if strings.ToLower(detail.Type) == SWR {
		nameIdentities := strings.SplitN(detail.Name, "/", 2)
		if len(nameIdentities) != 2 {
			return nil, errors.New(fmt.Sprintf("incorrect SWR image name %s found", detail.Name))
		}
		return NewSWRV2ImagePuller(h.user, h.password, h.baseFolder, nameIdentities[0], nameIdentities[1], h.logger, h.lxdClient)
	}
	return nil, errors.New(fmt.Sprintf("unsupported docker image type %s found", detail.Type))
}

func (h *Handler) pullingImage(index int, closeCh chan bool) {
	ctx, cancel := context.WithCancel(context.Background())
	readyCh := make(chan bool, 1)
	readyCh <- true
	canceled := false
	for {
		if canceled {
			return
		}
		select {
		case _, ok := <-closeCh:
			if !ok {
				h.logger.Info(fmt.Sprintf("close channel received, will quit load image for worker %d",
					index))
				cancel()
				canceled = true
				time.Sleep(time.Second * 5)
			}
		case <-readyCh:
			i := <-h.imageCh
			//loadLock.Lock()
			h.logger.Info(fmt.Sprintf("start to download image %s", i.Name))
			puller, err := h.GetImagePuller(i)
			if err != nil {
				//loadLock.Unlock()
				h.logger.Error(err.Error())
				readyCh <- true
				continue
			}
			puller.DownloadImage(ctx, readyCh)
			//loadLock.Unlock()
		}
	}
}

func (h *Handler) pushImageLoadTask() error {
	images, err := h.retrieveImages()
	if err != nil {
		log.Logger.Error(fmt.Sprintln("h.retrieveImages, err: ", err))
		return err
	}
	if len(h.imageCh)+len(images) > cap(h.imageCh) {
		h.logger.Warn(fmt.Sprintf("still have too many jobs [%d/%d] need to be finished, skip new arrangement",
			len(h.imageCh), cap(h.imageCh)))
		return nil
	}
	h.logger.Info(fmt.Sprintf("new image load tasks arranged, current jobs are [%d/%d]",
		len(h.imageCh)+len(images), cap(h.imageCh)))
	for _, image := range images {
		h.imageCh <- image
	}
	return nil
}

func (h *Handler) retrieveImages() ([]ImageDetail, error) {
	reqUrl, err := url.Parse(h.metaEndpoint)
	if err != nil {
		log.Logger.Error(fmt.Sprintln("h.metaEndpoint: ", h.metaEndpoint, ", err: ", err))
		return []ImageDetail{}, err
	}
	req, err := http.NewRequest("GET", reqUrl.String(), nil)
	ctx, _ := context.WithTimeout(context.Background(), time.Second*10)
	req.WithContext(ctx)
	if err != nil {
		log.Logger.Error(fmt.Sprintln("reqUrl: ", reqUrl.String(), ", err: ", err))
		return []ImageDetail{}, err
	}
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Logger.Error(fmt.Sprintln("resp: ", resp, ", err: ", err))
		return []ImageDetail{}, err
	}
	if resp.StatusCode != http.StatusOK {
		log.Logger.Error(fmt.Sprintln("resp.StatusCode: ", resp.StatusCode, ", err: ", err))
		return []ImageDetail{}, errors.New(fmt.Sprintf("request %s response code incorrect expected %d got %d",
			reqUrl.String(), http.StatusOK, resp.StatusCode))
	}
	decoder := json.NewDecoder(resp.Body)
	var imageResponse LXDImageResponse
	var imageList LXDImageList
	imageReList := make([]ImageDetail, 0)
	if err = decoder.Decode(&imageList); err != nil {
		log.Logger.Error(fmt.Sprintln("decoder.Decode(&imageResponse), err: ", err))
		return []ImageDetail{}, errors.New(fmt.Sprintf(
			"failed to get image meta info from api response: %s", err))
	}
	if len(imageList.Images) > 0 {
		for _, image := range imageList.Images {
			imageDetail := ImageDetail{}
			imageDetail.Name = image
			if strings.Contains(image, SWR) {
				imageDetail.Type = SWR
			} else {
				imageDetail.Type = DOCKER
			}
			imageReList = append(imageReList, imageDetail)
		}
		imageResponse.Images = imageReList
	}
	log.Logger.Info(fmt.Sprintln("imageResponse.Images: ", imageResponse.Images))
	return imageResponse.Images, nil
}
