//go:build !linux

package telemetry

import "errors"

var ErrUnsupported = errors.New("TCP_INFO collection is supported only on Linux")

type SystemProvider struct{}

func NewSystemProvider(bool) (TCPProvider, error)         { return nil, ErrUnsupported }
func (SystemProvider) Connections() ([]Connection, error) { return nil, ErrUnsupported }
