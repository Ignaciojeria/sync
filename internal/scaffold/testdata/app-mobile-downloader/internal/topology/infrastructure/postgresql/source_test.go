package postgresql

import (
	"context"
	"errors"
	"testing"

	topologyapp "gitinittest5/internal/topology/application"
)

type pingerStub struct {
	err error
}

func (p pingerStub) PingContext(context.Context) error {
	return p.err
}

func TestSourceListServicesNilDB(t *testing.T) {
	source := NewSource(nil)
	services, err := source.ListServices(context.Background())
	if err != nil {
		t.Fatalf("ListServices() error = %v", err)
	}
	if len(services) != 1 {
		t.Fatalf("len(services) = %d", len(services))
	}
	if services[0].Status != topologyapp.StatusOffline {
		t.Fatalf("status = %q", services[0].Status)
	}
}

func TestSourceListServicesHealthy(t *testing.T) {
	source := NewSource(pingerStub{})
	services, err := source.ListServices(context.Background())
	if err != nil {
		t.Fatalf("ListServices() error = %v", err)
	}
	if services[0].Status != topologyapp.StatusRunning {
		t.Fatalf("status = %q", services[0].Status)
	}
}

func TestSourceListServicesDegraded(t *testing.T) {
	source := NewSource(pingerStub{err: errors.New("down")})
	services, err := source.ListServices(context.Background())
	if err != nil {
		t.Fatalf("ListServices() error = %v", err)
	}
	if services[0].Status != topologyapp.StatusDegraded {
		t.Fatalf("status = %q", services[0].Status)
	}
}
