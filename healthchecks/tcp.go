package healthchecks

import (
	"crypto/tls"
	"fmt"
	hcconf "github.com/orange-cloudfoundry/gsloc-go-sdk/gsloc/api/config/healthchecks/v1"
	"io"
	"net"
	"time"
)

type TcpHealthCheck struct {
	hcConf     *hcconf.TcpHealthCheck
	tlsEnabled bool
	timeout    time.Duration
	tlsConf    *tls.Config
}

func NewTcpHealthCheck(hcConf *hcconf.TcpHealthCheck, timeout time.Duration, tlsEnabled bool, tlsConf *tls.Config) *TcpHealthCheck {
	return &TcpHealthCheck{
		hcConf:     hcConf,
		tlsEnabled: tlsEnabled,
		timeout:    timeout,
		tlsConf:    tlsConf,
	}
}

func (h *TcpHealthCheck) Check(host string) error {
	netConn, err := h.makeNetConn(host)
	if err != nil {
		return err
	}
	defer netConn.Close()

	if h.hcConf.Send != nil {
		_, err = netConn.Write(h.hcConf.Send.GetData())
		if err != nil {
			return err
		}
	}

	for _, toReceive := range h.hcConf.GetReceive() {
		err := netConn.SetReadDeadline(time.Now().Add(h.timeout))
		if err != nil {
			return err
		}
		buf := make([]byte, len(toReceive.GetData()))
		n, err := io.ReadFull(netConn, buf)
		if err != nil {
			return fmt.Errorf("failed to read %d bytes: %v", len(toReceive.GetData()), err)
		}
		got := buf[0:n]
		if string(got) != string(toReceive.GetData()) {
			return fmt.Errorf("expected %s, got %s", string(toReceive.GetData()), string(got))
		}
	}
	return nil
}

func (h *TcpHealthCheck) makeNetConn(host string) (net.Conn, error) {
	dialer := &net.Dialer{
		Timeout: h.timeout,
	}
	if h.tlsEnabled {
		return tls.DialWithDialer(dialer, "tcp", host, h.tlsConf)
	}
	return dialer.Dial("tcp", host)
}
