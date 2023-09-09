package proxmetrics

import (
	"fmt"
	"net/http"
	"net/url"
)

func ErrNoInstanceFound(instanceUuid string) *ErrFetch {
	return &ErrFetch{
		Code:    http.StatusNotFound,
		Message: "Cannot found instance " + instanceUuid,
	}
}

func ErrNoEndpointFound(appIdOrPath, endpoint string) *ErrFetch {
	appIdOrPathTmp, err := url.PathUnescape(appIdOrPath)
	if err == nil {
		appIdOrPath = appIdOrPathTmp
	}
	return &ErrFetch{
		Code: http.StatusNotAcceptable,
		Message: fmt.Sprintf(
			"Cannot found endpoint '%s' for app with id or path '%s', please create one", endpoint, appIdOrPath,
		),
	}
}

type ErrFetch struct {
	Code    int
	Message string
}

func (e ErrFetch) Error() string {
	return fmt.Sprintf("%d %s\n", e.Code, e.Message)
}
