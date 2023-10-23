package healthchecks

import (
	"crypto/sha256"
	"crypto/subtle"
	"fmt"
	"github.com/gorilla/mux"
	hcconf "github.com/orange-cloudfoundry/gsloc-go-sdk/gsloc/api/config/healthchecks/v1"
	"github.com/orange-cloudfoundry/gsloc/config"
	log "github.com/sirupsen/logrus"
	"google.golang.org/protobuf/encoding/protojson"
	"io"
	"net/http"
	"strings"
	"sync"
)

type HcHandler struct {
	disabledEntIp *sync.Map
	cnf           *config.HealthCheckConfig
}

func NewHcHandler(cnf *config.HealthCheckConfig) *HcHandler {
	return &HcHandler{
		disabledEntIp: &sync.Map{},
		cnf:           cnf,
	}
}

func (h *HcHandler) DisableEntryIp(fqdn, ip string) {
	log.Tracef(fmt.Sprintf("Disabling %s-%s", fqdn, ip))
	h.disabledEntIp.Store(fmt.Sprintf("%s-%s", fqdn, ip), struct{}{})
}

func (h *HcHandler) EnableEntryIp(fqdn, ip string) {
	log.Tracef(fmt.Sprintf("Enabling %s-%s", fqdn, ip))
	h.disabledEntIp.Delete(fmt.Sprintf("%s-%s", fqdn, ip))
}

func (h *HcHandler) checkAuth(req *http.Request) bool {
	if h.cnf.HealthcheckAuth == nil {
		return true
	}
	username, password, ok := req.BasicAuth()
	if !ok {
		return false
	}
	usernameHash := sha256.Sum256([]byte(username))
	passwordHash := sha256.Sum256([]byte(password))
	expectedUsernameHash := sha256.Sum256([]byte(h.cnf.HealthcheckAuth.Username))
	expectedPasswordHash := sha256.Sum256([]byte(h.cnf.HealthcheckAuth.Password))

	usernameMatch := (subtle.ConstantTimeCompare(usernameHash[:], expectedUsernameHash[:]) == 1)
	passwordMatch := (subtle.ConstantTimeCompare(passwordHash[:], expectedPasswordHash[:]) == 1)

	return usernameMatch && passwordMatch
}

func (h *HcHandler) isFromLocalhost(req *http.Request) bool {
	if strings.HasPrefix(req.RemoteAddr, "127.0.0.1:") {
		return true
	}
	if strings.HasPrefix(req.RemoteAddr, "[::1]:") {
		return true
	}
	return false
}

func (h *HcHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if h.cnf.AllowOnlyLocalhost && !h.isFromLocalhost(req) {
		http.Error(w, "only localhost allowed", http.StatusForbidden)
		return
	}
	if !h.isFromLocalhost(req) && !h.checkAuth(req) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
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

	hcker, err := MakeHealthCheck(hcDef, fqdn, h.cnf.Plugins)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	host := fmt.Sprintf("%s:%d", ip, hcDef.GetPort())
	err = hcker.Check(host)
	if err != nil {
		http.Error(w, err.Error(), http.StatusExpectationFailed)
		return
	}
	w.WriteHeader(http.StatusOK)
}
