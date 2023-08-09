package testhelpers

import (
	"github.com/onsi/gomega"
	"sync/atomic"
)

func EventuallyAtomic(actual *int64) gomega.AsyncAssertion {
	return gomega.Eventually(func() int {
		return int(atomic.LoadInt64(actual))
	})
}
