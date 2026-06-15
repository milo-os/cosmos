package provider

import (
	"context"
	"fmt"
	"sync"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Pool manages gRPC connections to remote BGP provider agents keyed by endpoint.
// It also maintains a name->endpoint index so other reconcilers can look up
// providers by BGPProvider resource name without knowing the endpoint.
type Pool struct {
	mu    sync.RWMutex
	conns map[string]*poolEntry // key: endpoint "host:port"
	names map[string]string     // key: BGPProvider resource name, value: endpoint
}

type poolEntry struct {
	conn     *grpc.ClientConn
	provider Provider
}

// NewPool creates an empty Pool.
func NewPool() *Pool {
	return &Pool{
		conns: make(map[string]*poolEntry),
		names: make(map[string]string),
	}
}

// Get returns the Provider for endpoint, dialing insecure if not already connected.
func (p *Pool) Get(endpoint string) (Provider, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if e, ok := p.conns[endpoint]; ok {
		return e.provider, nil
	}
	conn, err := grpc.NewClient(endpoint, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("dial %s: %w", endpoint, err)
	}
	e := &poolEntry{conn: conn, provider: NewGRPCProvider(conn)}
	p.conns[endpoint] = e
	return e.provider, nil
}

// Register associates a BGPProvider resource name with an endpoint.
func (p *Pool) Register(name, endpoint string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.names[name] = endpoint
}

// Unregister removes the name->endpoint association.
func (p *Pool) Unregister(name string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.names, name)
}

// GetByName returns the Provider for the given BGPProvider resource name.
func (p *Pool) GetByName(name string) (Provider, bool) {
	p.mu.RLock()
	endpoint, ok := p.names[name]
	if !ok {
		p.mu.RUnlock()
		return nil, false
	}
	e, ok := p.conns[endpoint]
	p.mu.RUnlock()
	if !ok {
		return nil, false
	}
	return e.provider, true
}

// Release closes and removes the connection for endpoint.
func (p *Pool) Release(endpoint string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if e, ok := p.conns[endpoint]; ok {
		if e.conn != nil {
			_ = e.conn.Close()
		}
		delete(p.conns, endpoint)
	}
}

// Close closes all connections.
func (p *Pool) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for ep, e := range p.conns {
		if e.conn != nil {
			_ = e.conn.Close()
		}
		delete(p.conns, ep)
	}
}

// SetForTest inserts a pre-built provider under name without opening a real gRPC
// connection. Intended for unit tests only.
func (p *Pool) SetForTest(name string, prov Provider) {
	p.mu.Lock()
	defer p.mu.Unlock()
	// Use the name as the synthetic endpoint key so Release/Unregister still work.
	ep := "test://" + name
	p.conns[ep] = &poolEntry{conn: nil, provider: prov}
	p.names[name] = ep
}

// Start implements sigs.k8s.io/controller-runtime/pkg/manager.Runnable.
// It blocks until ctx is cancelled, then closes all connections.
func (p *Pool) Start(ctx context.Context) error {
	<-ctx.Done()
	p.Close()
	return nil
}
