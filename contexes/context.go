package contexes

import (
	"context"
	"github.com/miekg/dns"
)

type gslocCtxKey int

const (
	DNSMsg gslocCtxKey = iota
	RemoteAddr
	FromLocalhost
)

func SetDNSMsg(ctx context.Context, msg *dns.Msg) context.Context {
	return context.WithValue(ctx, DNSMsg, msg)
}

func GetDNSMsg(ctx context.Context) *dns.Msg {
	val := ctx.Value(DNSMsg)
	if val == nil {
		return nil
	}
	return val.(*dns.Msg)
}

func SetRemoteAddr(ctx context.Context, addr string) context.Context {
	return context.WithValue(ctx, RemoteAddr, addr)
}

func GetRemoteAddr(ctx context.Context) string {
	val := ctx.Value(RemoteAddr)
	if val == nil {
		return ""
	}
	return val.(string)
}
