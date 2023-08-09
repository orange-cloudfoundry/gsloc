package healthchecks

import (
	"crypto/tls"
	"fmt"
	"github.com/gorilla/mux"
	hcconf "github.com/orange-cloudfoundry/gsloc-go-sdk/gsloc/api/config/healthchecks/v1"
	"google.golang.org/protobuf/encoding/protojson"
	"io"
	"net/http"
	"sync"
)

type HcHandler struct {
	tlsConf       *tls.Config
	disabledEntIp *sync.Map
}

func NewHcHandler(tlsConf *tls.Config) *HcHandler {
	return &HcHandler{
		tlsConf:       tlsConf,
		disabledEntIp: &sync.Map{},
	}
}

func (h *HcHandler) DisableEntryIp(fqdn, ip string) {
	h.disabledEntIp.Store(fmt.Sprintf("%s-%s", fqdn, ip), struct{}{})
}

func (h *HcHandler) EnableEntryIp(fqdn, ip string) {
	h.disabledEntIp.Delete(fmt.Sprintf("%s-%s", fqdn, ip))
}

func (h *HcHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	b, err := io.ReadAll(req.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	vars := mux.Vars(req)
	fqdn := vars["fqdn"]
	if fqdn == "" {
		http.Error(w, "fqdn is empty", http.StatusBadRequest)
		return
	}
	ip := vars["ip"]
	if ip == "" {
		http.Error(w, "ip is empty", http.StatusBadRequest)
		return
	}

	_, isDisabled := h.disabledEntIp.Load(fmt.Sprintf("%s-%s", fqdn, ip))
	if isDisabled {
		http.Error(w, "disabled entry", http.StatusGone)
		return
	}

	hcDef := &hcconf.HealthCheck{}
	err = protojson.Unmarshal(b, hcDef)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	hcker := MakeHealthCheck(hcDef, h.tlsConf)
	host := fmt.Sprintf("%s:%d", ip, hcDef.GetPort())
	err = hcker.Check(host)
	if err != nil {
		http.Error(w, err.Error(), http.StatusExpectationFailed)
		return
	}
	w.WriteHeader(http.StatusOK)
}
