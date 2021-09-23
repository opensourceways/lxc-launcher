package image

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/opensourceways/lxc-launcher/util"
	"go.etcd.io/etcd/client/pkg/v3/fileutil"
	"go.uber.org/atomic"
	"go.uber.org/zap"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var (
	DEFAULT_TIMEOUT       = 10
	DOCKER_CONTENT_DIGEST = "Docker-Content-Digest"
	MANIFEST_DIGEST       = "manifest.digest"
	httpClient            *http.Client
	ROOTFS_DIR            = "rootfs"
)

type RegistryType int32

const (
	SWRRegistry RegistryType = iota
	DockerRegistry
)

type tokenResponse struct {
	Token     string    `json:"token"`
	ExpiresIn int       `json:"expires_in"`
	IssuedAt  time.Time `json:"issued_at"`
}

type SWRManifestResponse struct {
	Layers []SWRManifestLayers `json:"layers"`
}

type SWRManifestLayers struct {
	MediaType string `json:"mediaType"`
	Type      string `json:"type"`
	Digest    string `json:"digest"`
}

type DockerManifestResponse struct {
	Layers []DockerManifestLayers `json:"fsLayers"`
}

type DockerManifestLayers struct {
	BlobSum string `json:"blobSum"`
}

type Puller struct {
	username         string
	password         string
	authEndpoint     string
	registryEndpoint string
	registryHost     string
	serviceName      string
	registryToken    string
	tokenExpiration  time.Time
	imageName        string
	imageTag         string
	logger           *zap.Logger
	imageFolder      string
	registryType     RegistryType
	canceled         *atomic.Bool
	imageDigest      string
}

func newImagePuller(username, password, baseFolder, imageFullName string, logger *zap.Logger) (*Puller, error) {
	if !fileutil.Exist(baseFolder) {
		return nil, errors.New(fmt.Sprintf("base folder %s not existed", baseFolder))
	}
	puller := &Puller{
		username: username,
		password: password,
		logger:   logger,
		canceled:  atomic.NewBool(false),
	}

	imageIDs := strings.Split(imageFullName, ":")
	if len(imageIDs) > 2 {
		return nil, errors.New(fmt.Sprintf("image ID %s incorreect", imageFullName))
	} else if len(imageIDs) == 1 {
		puller.imageName = imageFullName
		puller.imageTag = "latest"
		puller.imageFolder = path.Join(baseFolder, util.GetImagePath(fmt.Sprintf("%s:latest", imageFullName)))
	} else {
		puller.imageName = imageIDs[0]
		puller.imageTag = imageIDs[1]
		puller.imageFolder = path.Join(baseFolder, util.GetImagePath(imageFullName))
	}

	httpClient = &http.Client{
		Transport: &http.Transport{
			Proxy: func(req *http.Request) (*url.URL, error) {
				if !puller.tokenValid() {
					if err := puller.refreshToken(); err != nil {
						return nil, err
					}
				}
				// only add authorization when request registry host.
				if strings.Contains(req.URL.String(), puller.registryHost) {
					req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", puller.registryToken))
				}
				return nil, nil
			},
		},
	}
	return puller, nil
}

func (p *Puller) Cancel() {
	p.canceled.Store(true)
}

func (p *Puller) tokenValid() bool {
	if len(p.registryToken) == 0 {
		return false
	}
	now := time.Now()
	if now.After(p.tokenExpiration) {
		return false
	}
	return true
}

func (p *Puller) refreshToken() error {
	realm := fmt.Sprintf("%s?service=%s&scope=repository:%s:pull", strings.TrimRight(p.authEndpoint,
		"/"), p.serviceName, p.imageName)
	p.logger.Info(fmt.Sprintf("start to refresh registry token for image %s:%s", p.imageName, p.imageTag))
	reqUrl, err := url.Parse(realm)
	if err != nil {
		return err
	}
	req, err := http.NewRequest("GET", reqUrl.String(), nil)
	if err != nil {
		return err
	}
	if len(p.username) != 0 && len(p.password) != 0 {
		req.SetBasicAuth(p.username, p.password)
	}
	cl := &http.Client{
		Timeout: time.Duration(DEFAULT_TIMEOUT) * time.Second,
	}
	resp, err := cl.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := ioutil.ReadAll(resp.Body)
		return errors.New(fmt.Sprintf("failed to refresh registry token: %s", string(bodyBytes)))
	}
	decoder := json.NewDecoder(resp.Body)
	var tr tokenResponse
	if err = decoder.Decode(&tr); err != nil {
		return errors.New(fmt.Sprintf("failed to decode registry token: %s", err))
	}
	p.registryToken = tr.Token
	if tr.IssuedAt.IsZero() {
		tr.IssuedAt = time.Now().UTC()
	}
	p.tokenExpiration = tr.IssuedAt.Add(time.Duration(tr.ExpiresIn) * time.Second)
	return nil
}

func NewSWRV2ImagePuller(username, password, baseFolder, region, imageFullName string, logger *zap.Logger) (*Puller, error) {
	puller, err := newImagePuller(username, password, baseFolder, imageFullName, logger)
	if err != nil {
		return nil, err
	}
	puller.authEndpoint = fmt.Sprintf("https://%s/swr/auth/v2/registry/auth", region)
	puller.serviceName = "dockyard"
	puller.registryEndpoint = fmt.Sprintf("https://%s/v2", region)
	puller.registryType = SWRRegistry
	url, err := url.Parse(puller.registryEndpoint)
	if err != nil {
		return nil, err
	}
	puller.registryHost = url.Host
	if err := puller.refreshToken(); err != nil {
		return nil, err
	}
	return puller, nil
}

func NewDockerIOV2ImagePuller(username, password, baseFolder, imageFullName string, logger *zap.Logger) (*Puller, error) {
	puller, err := newImagePuller(username, password, baseFolder, imageFullName, logger)
	if err != nil {
		return nil, err
	}
	puller.authEndpoint = "https://auth.docker.io/token"
	puller.registryEndpoint = "https://registry-1.docker.io/v2"
	puller.serviceName = "registry.docker.io"
	puller.registryType = DockerRegistry
	url, err := url.Parse(puller.registryEndpoint)
	if err != nil {
		return nil, err
	}
	puller.registryHost = url.Host
	if err := puller.refreshToken(); err != nil {
		return nil, err
	}
	return puller, nil
}

func (p *Puller) DownloadImage(ctx context.Context, finishedCh chan bool) {
	defer func() {
		finishedCh <- true
	}()
	if fileutil.Exist(p.imageFolder) {
		digest := filepath.Join(p.imageFolder, MANIFEST_DIGEST)
		if fileutil.Exist(digest) {
			currentDigest, err := util.ReadContent(digest)
			if err != nil {
				p.logger.Error(err.Error())
				return
			}
			p.imageDigest, err = p.getImageManifestDigest(ctx)
			if err != nil {
				p.logger.Error(err.Error())
				return
			}
			if currentDigest == p.imageDigest {
				p.logger.Info(fmt.Sprintf("image %s:%s unchanged, skip syncing", p.imageName, p.imageTag))
				return
			}
		}
		//delete folder due to digest file missing
		err := os.RemoveAll(p.imageFolder)
		if err != nil {
			p.logger.Error(err.Error())
		}
	}
	//create rootfs inside of image folder
	err := fileutil.CreateDirAll(path.Join(p.imageFolder, ROOTFS_DIR))
	if err != nil {
		p.logger.Error(err.Error())
		return
	}
	//create and download images
	p.logger.Info(fmt.Sprintf("start to download image %s:%s and load into lxd", p.imageName, p.imageTag))
	blobs, err := p.getImageBlobs(ctx)
	if err != nil {
		p.logger.Error(err.Error())
		return
	}
	var wg sync.WaitGroup
	resChannel := make(chan string, len(blobs))
	var results []string
	go func() {
		for {
			select {
			case err, ok := <-resChannel:
				if !ok {
					p.logger.Info("blob download channel going to shutdown, task finished")
					return
				} else {
					results = append(results, err)
				}

			}
		}
	}()
	for i, blob := range blobs {
		wg.Add(1)
		index := fmt.Sprintf("[%d/%d]", i+1, len(blobs))
		go p.downloadBlob(ctx, index, blob, &wg, resChannel)
	}
	wg.Wait()
	close(resChannel)
	if len(results) != 0 {
		p.logger.Error(fmt.Sprintf("image %s:%s, (%d/%d) blobs download failed, the first error is %s",
			p.imageName, p.imageTag, len(results), len(blobs), results[0]))
		return
	}
	//write digest
	if len(p.imageDigest) == 0 {
		p.imageDigest, err = p.getImageManifestDigest(ctx)
		if err != nil {
			p.logger.Warn(fmt.Sprintf("unable to collect image digest from registry, %s", err.Error()))
		}
	}
	if len(p.imageDigest) != 0 {
		err = util.WriteContent(filepath.Join(p.imageFolder, MANIFEST_DIGEST), p.imageDigest)
		if err != nil {
			p.logger.Warn(fmt.Sprintf("unable to write image digest into file %s, %s",
				filepath.Join(p.imageFolder, MANIFEST_DIGEST), err.Error()))
		}
	}
	//load images into lxd
	p.logger.Info(fmt.Sprintf("download image %s:%s successfully finished", p.imageName, p.imageTag))

}

func (p *Puller) loadLXDImages() error {
	return nil
}

func (p *Puller) getImageBlobs(ctx context.Context) ([]string, error) {
	var blobs []string
	manifest := fmt.Sprintf("%s/%s/manifests/%s", p.registryEndpoint, p.imageName, p.imageTag)
	reqUrl, err := url.Parse(manifest)
	if err != nil {
		return blobs, err
	}
	req, err := http.NewRequest("GET", reqUrl.String(), nil)
	req.WithContext(ctx)
	if err != nil {
		return blobs, err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return blobs, err
	}
	decoder := json.NewDecoder(resp.Body)
	if p.registryType == SWRRegistry {
		var mr SWRManifestResponse
		if err = decoder.Decode(&mr); err != nil {
			return blobs, errors.New(fmt.Sprintf("failed to decode swr manifest response: %s", err))
		}
		for _, l := range mr.Layers {
			blobs = append(blobs, l.Digest)
		}
	} else {
		var dr DockerManifestResponse
		if err = decoder.Decode(&dr); err != nil {
			return blobs, errors.New(fmt.Sprintf("failed to decode docker manifest response: %s", err))
		}
		for _, l := range dr.Layers {
			blobs = append(blobs, l.BlobSum)
		}
	}
	return blobs, nil

}

func (p *Puller) downloadBlob(ctx context.Context, index string, blobID string, wg *sync.WaitGroup, result chan string) {
	p.logger.Info(fmt.Sprintf(
		"start to download blob %s %s for image %s:%s", index, blobID, p.imageName, p.imageTag))
	defer wg.Done()
	raw := fmt.Sprintf("%s/%s/blobs/%s", strings.TrimRight(p.registryEndpoint,
		"/"), p.imageName, blobID)
	reqUrl, err := url.Parse(raw)
	if err != nil {
		result <- err.Error()
		return
	}
	req, err := http.NewRequest("GET", reqUrl.String(), nil)
	req.WithContext(ctx)
	if err != nil {
		result <- err.Error()
		return
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		result <- err.Error()
		return
	}
	if resp.StatusCode != http.StatusOK {
		buf := new(strings.Builder)
		_, _ = io.Copy(buf, resp.Body)
		result <- fmt.Sprintf("request %s response code incorrect expected %d got %d and response %s",
			reqUrl.String(), http.StatusOK, resp.StatusCode, buf.String())
		return
	}
	gReader, err := gzip.NewReader(resp.Body)
	if err != nil {
		result <- err.Error()
		return
	}
	defer gReader.Close()
	tr := tar.NewReader(gReader)
	finished := make(chan bool, 1)
	defer close(finished)
	go func() {
		for {
			select {
				case <- finished:
					return
				case <- ctx.Done():
					p.Cancel()
					return
			}
		}
	}()
	for {
		if p.canceled.Load() == true {
			result <- fmt.Sprintf(
				"pull image blob %s %s for image %s:%s canceled.", index, blobID, p.imageName, p.imageTag)
			return
		}
		hdr, err := tr.Next()
		switch {
		case err == io.EOF:
			return
		case err != nil:
			result <- err.Error()
			return
		case hdr == nil:
			continue
		}
		dstFileDir := filepath.Join(p.imageFolder, ROOTFS_DIR, hdr.Name)
		switch hdr.Typeflag {
		case tar.TypeDir:
			if b := fileutil.Exist(dstFileDir); !b {
				if err := os.MkdirAll(dstFileDir, 0775); err != nil {
					result <- err.Error()
					return
				}
			}
		case tar.TypeReg:
			file, err := os.OpenFile(dstFileDir, os.O_CREATE|os.O_RDWR, os.FileMode(hdr.Mode))
			if err != nil {
				result <- err.Error()
				return
			}
			_, err = io.Copy(file, tr)
			if err != nil {
				result <- err.Error()
				return
			}
			file.Close()
		}
	}
}

func (p *Puller) getImageManifestDigest(ctx context.Context) (string, error) {
	raw := fmt.Sprintf("%s/%s/manifests/%s", strings.TrimRight(p.registryEndpoint,
		"/"), p.imageName, p.imageTag)
	p.logger.Info(fmt.Sprintf("start to fetch manifest digest for image %s:%s", p.imageName, p.imageTag))
	reqUrl, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequest("HEAD", reqUrl.String(), nil)
	req.WithContext(ctx)
	if err != nil {
		return "", err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", errors.New(fmt.Sprintf("request %s response code incorrect expected %d got %d",
			reqUrl.String(), http.StatusOK, resp.StatusCode))
	}
	digest := resp.Header.Get(DOCKER_CONTENT_DIGEST)
	if len(digest) == 0 {
		digest = resp.Header.Get(strings.ToLower(DOCKER_CONTENT_DIGEST))
	}
	if len(digest) == 0 {
		return "", errors.New("failed to get manifest digest")
	}
	return digest, nil
}
