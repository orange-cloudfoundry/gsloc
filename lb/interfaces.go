package lb

import (
	"context"
	"github.com/orange-cloudfoundry/gsloc-go-sdk/gsloc/api/config/entries/v1"
)

type MemberType int

const (
	All MemberType = iota
	Ipv4
	Ipv6
)

type Loadbalancer interface {
	Next(ctx context.Context, memberType MemberType) (*entries.Member, error)
	Reset() error

	Name() string
}
