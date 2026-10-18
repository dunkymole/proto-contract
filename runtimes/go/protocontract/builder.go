package protocontract

import (
	"context"
	"fmt"
	"google.golang.org/grpc"
)

// Bind attaches the contract to this client only; the connection remains caller-owned.
func (c *Client) Bind(conn grpc.ClientConnInterface) grpc.ClientConnInterface {
	return &boundConnection{conn, c}
}

type boundConnection struct {
	grpc.ClientConnInterface
	contract *Client
}

func (c *boundConnection) Invoke(ctx context.Context, method string, req, reply any, opts ...grpc.CallOption) error {
	return c.ClientConnInterface.Invoke(c.contract.stamp(ctx, method), method, req, reply, opts...)
}
func (c *boundConnection) NewStream(ctx context.Context, desc *grpc.StreamDesc, method string, opts ...grpc.CallOption) (grpc.ClientStream, error) {
	return c.ClientConnInterface.NewStream(c.contract.stamp(ctx, method), desc, method, opts...)
}

// ServerBuilder couples service registration with contract registration.
// Configure it before Build; it is not safe for concurrent mutation.
type ServerBuilder struct {
	contracts     map[string]*Server
	registrations []registration
	exemptions    []string
	built         bool
}
type registration struct {
	desc *grpc.ServiceDesc
	impl any
}
type collector struct{ registrations []registration }

func (c *collector) RegisterService(desc *grpc.ServiceDesc, impl any) {
	c.registrations = append(c.registrations, registration{desc, impl})
}
func NewServerBuilder(exemptions ...string) *ServerBuilder {
	return &ServerBuilder{contracts: map[string]*Server{}, exemptions: append([]string(nil), exemptions...)}
}
func (b *ServerBuilder) Register(contract *Server, register func(grpc.ServiceRegistrar)) error {
	if b.built {
		return fmt.Errorf("server builder already built")
	}
	if contract == nil || contract.service == "" {
		return fmt.Errorf("service-scoped contract required")
	}
	c := &collector{}
	register(c)
	if len(c.registrations) != 1 || c.registrations[0].desc == nil || c.registrations[0].desc.ServiceName != contract.service {
		return fmt.Errorf("registration does not match contract service %s", contract.service)
	}
	for _, name := range b.exemptions {
		if name == contract.service {
			return fmt.Errorf("service cannot be protected and exempt")
		}
	}
	for _, r := range b.registrations {
		if r.desc.ServiceName == contract.service {
			return fmt.Errorf("duplicate service %s", contract.service)
		}
	}
	b.contracts[contract.service] = contract
	b.registrations = append(b.registrations, c.registrations...)
	return nil
}

// Build reserves the single unary/stream interceptor slots for strict enforcement,
// which gRPC runs before all chain interceptors. Supply application interceptors
// with grpc.ChainUnaryInterceptor and grpc.ChainStreamInterceptor; gRPC rejects
// attempts to replace the reserved single slots as duplicate configuration.
// Later raw registrations are still denied globally.
func (b *ServerBuilder) Build(opts ...grpc.ServerOption) (*grpc.Server, error) {
	if b.built {
		return nil, fmt.Errorf("server builder already built")
	}
	strict, err := NewStrictServer(b.contracts, b.exemptions)
	if err != nil {
		return nil, err
	}
	opts = append(opts, grpc.UnaryInterceptor(strict.Unary()), grpc.StreamInterceptor(strict.Stream()))
	server := grpc.NewServer(opts...)
	for _, r := range b.registrations {
		server.RegisterService(r.desc, r.impl)
	}
	if err := strict.ValidateRegisteredServices(server); err != nil {
		server.Stop()
		return nil, err
	}
	b.built = true
	return server, nil
}
