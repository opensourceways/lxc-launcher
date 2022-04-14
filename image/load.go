package image

import (
	"context"
	"errors"
	"strings"
)

func LoadImage(im string, imageHandel *Handler) error {
	var imageDetail = ImageDetail{
		Name: im,
	}

	if strings.Contains(im, SWR) {
		imageDetail.Type = SWR
	} else if strings.Contains(im, DOCKER) {
		imageDetail.Type = DOCKER
	} else {
		return errors.New("image type wrong")
	}

	pull, err := imageHandel.GetImagePuller(imageDetail)
	if err != nil {
		return err
	}

	pull.DownloadImage(context.Background(), imageHandel.closeCh)

	return nil
}
