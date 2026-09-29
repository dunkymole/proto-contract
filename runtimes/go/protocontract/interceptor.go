package protocontract

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const MetadataKey = "x-proto-contract"
const MaxMetadataLength = 128
const maxComponent = int64(2147483647)

var apiPattern = regexp.MustCompile(`^[A-Za-z0-9._:/-]{1,64}$`)
var versionPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)

// Client binds one contract to one protobuf service. Reusing a Client across
// services is safe: calls outside Service are left unchanged.
type Client struct{ api, version, value, service string }

func NewClient(api, version, service string) (*Client, error) {
	value, err := validateConfiguration(api, version)
	if err != nil {
		return nil, err
	}
	if service != "" && !validService(service) {
		return nil, fmt.Errorf("invalid protobuf service name")
	}
	return &Client{api: api, version: version, value: value, service: service}, nil
}

func validateConfiguration(api, version string) (string, error) {
	if !apiPattern.MatchString(api) {
		return "", fmt.Errorf("invalid contract API identifier")
	}
	if _, err := parse(version); err != nil {
		return "", err
	}
	value := api + "@" + version
	if len(value) > MaxMetadataLength {
		return "", fmt.Errorf("contract metadata exceeds 128 ASCII bytes")
	}
	return value, nil
}

var servicePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*(\.[A-Za-z_][A-Za-z0-9_]*)*$`)

func validService(s string) bool { return servicePattern.MatchString(s) }
func (c *Client) applies(method string) bool {
	return c.service == "" || strings.HasPrefix(method, "/"+c.service+"/")
}

func (c *Client) stamp(ctx context.Context, method string) context.Context {
	if !c.applies(method) {
		return ctx
	}
	md, _ := metadata.FromOutgoingContext(ctx)
	md = md.Copy()
	md.Delete(MetadataKey)
	md.Set(MetadataKey, c.value)
	return metadata.NewOutgoingContext(ctx, md)
}

func (c *Client) Unary() grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		return invoker(c.stamp(ctx, method), method, req, reply, cc, opts...)
	}
}
func (c *Client) Stream() grpc.StreamClientInterceptor {
	return func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
		return streamer(c.stamp(ctx, method), desc, cc, method, opts...)
	}
}

// UnaryClient and StreamClient retain the simple API while validating eagerly.
// Invalid static configuration panics at interceptor construction, before RPCs.
func UnaryClient(api, version string) grpc.UnaryClientInterceptor {
	c, err := NewClient(api, version, "")
	if err != nil {
		panic(err)
	}
	return c.Unary()
}
func StreamClient(api, version string) grpc.StreamClientInterceptor {
	c, err := NewClient(api, version, "")
	if err != nil {
		panic(err)
	}
	return c.Stream()
}

type Server struct{ api, version, service string }

func NewServer(api, version, service string) (*Server, error) {
	if _, err := validateConfiguration(api, version); err != nil {
		return nil, err
	}
	if service != "" && !validService(service) {
		return nil, fmt.Errorf("invalid protobuf service name")
	}
	return &Server{api: api, version: version, service: service}, nil
}
func (s *Server) applies(method string) bool {
	return s.service == "" || strings.HasPrefix(method, "/"+s.service+"/")
}
func (s *Server) check(ctx context.Context, method string) error {
	if !s.applies(method) {
		return nil
	}
	md, _ := metadata.FromIncomingContext(ctx)
	return CompatibleValues(s.api, md.Get(MetadataKey), s.version)
}
func (s *Server) Unary() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if err := s.check(ctx, info.FullMethod); err != nil {
			return nil, status.Error(codes.FailedPrecondition, err.Error())
		}
		return handler(ctx, req)
	}
}
func (s *Server) Stream() grpc.StreamServerInterceptor {
	return func(srv any, stream grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		if err := s.check(stream.Context(), info.FullMethod); err != nil {
			return status.Error(codes.FailedPrecondition, err.Error())
		}
		return handler(srv, stream)
	}
}
func UnaryServer(api, version string) grpc.UnaryServerInterceptor {
	s, err := NewServer(api, version, "")
	if err != nil {
		panic(err)
	}
	return s.Unary()
}
func StreamServer(api, version string) grpc.StreamServerInterceptor {
	s, err := NewServer(api, version, "")
	if err != nil {
		panic(err)
	}
	return s.Stream()
}

func Compatible(api, offered, serverVersion string) error {
	return CompatibleValues(api, []string{offered}, serverVersion)
}
func CompatibleValues(api string, offeredValues []string, serverVersion string) error {
	if _, err := validateConfiguration(api, serverVersion); err != nil {
		return err
	}
	if len(offeredValues) == 0 {
		return fmt.Errorf("missing x-proto-contract metadata")
	}
	if len(offeredValues) != 1 {
		return fmt.Errorf("duplicate x-proto-contract metadata")
	}
	offered := offeredValues[0]
	if len(offered) > MaxMetadataLength {
		return fmt.Errorf("invalid x-proto-contract metadata")
	}
	parts := strings.Split(offered, "@")
	if len(parts) != 2 || !apiPattern.MatchString(parts[0]) || parts[0] != api {
		return fmt.Errorf("expected contract API@MAJOR.MINOR.PATCH")
	}
	c, err := parse(parts[1])
	if err != nil {
		return err
	}
	s, _ := parse(serverVersion)
	if c[0] != s[0] || c[1] > s[1] {
		return fmt.Errorf("incompatible contract: client %s, server %s", parts[1], serverVersion)
	}
	return nil
}
func parse(v string) ([3]int64, error) {
	var out [3]int64
	if !versionPattern.MatchString(v) {
		return out, fmt.Errorf("invalid semantic version %q", v)
	}
	p := strings.Split(v, ".")
	for i := range p {
		n, e := strconv.ParseInt(p[i], 10, 64)
		if e != nil || n > maxComponent {
			return out, fmt.Errorf("invalid semantic version %q", v)
		}
		out[i] = n
	}
	return out, nil
}

// ValidateServiceCoverage enforces startup coverage: every registered service
// must have a contract or an explicit exemption. Names are fully-qualified.
func ValidateServiceCoverage(registered, protected, exemptions []string) error {
	covered := map[string]bool{}
	for _, name := range protected {
		covered[name] = true
	}
	for _, name := range exemptions {
		covered[name] = true
	}
	var missing []string
	for _, name := range registered {
		if !covered[name] {
			missing = append(missing, name)
		}
	}
	if len(missing) != 0 {
		return fmt.Errorf("contract service coverage incomplete: unprotected=%s", strings.Join(missing, ","))
	}
	return nil
}

// StrictServer owns the service dispatch table and fails closed for every
// registered RPC whose service is neither protected nor explicitly exempt.
type StrictServer struct {
	contracts  map[string]*Server
	exemptions map[string]bool
}

func NewStrictServer(contracts map[string]*Server, exemptions []string) (*StrictServer, error) {
	copyContracts := make(map[string]*Server, len(contracts))
	for name, server := range contracts {
		if !validService(name) || server == nil || server.service != name {
			return nil, fmt.Errorf("contract interceptor is not bound to service %s", name)
		}
		copyContracts[name] = server
	}
	exempt := make(map[string]bool, len(exemptions))
	for _, name := range exemptions {
		if !validService(name) {
			return nil, fmt.Errorf("invalid protobuf service name %s", name)
		}
		if copyContracts[name] != nil {
			return nil, fmt.Errorf("service %s cannot be both protected and exempt", name)
		}
		exempt[name] = true
	}
	return &StrictServer{copyContracts, exempt}, nil
}
func (s *StrictServer) service(method string) (string, *Server, bool) {
	if !strings.HasPrefix(method, "/") {
		return "", nil, false
	}
	name, _, ok := strings.Cut(strings.TrimPrefix(method, "/"), "/")
	return name, s.contracts[name], ok
}
func (s *StrictServer) Unary() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		name, contract, ok := s.service(info.FullMethod)
		if !ok || (contract == nil && !s.exemptions[name]) {
			return nil, status.Error(codes.FailedPrecondition, "unprotected service registration")
		}
		if contract == nil {
			return handler(ctx, req)
		}
		return contract.Unary()(ctx, req, info, handler)
	}
}
func (s *StrictServer) Stream() grpc.StreamServerInterceptor {
	return func(srv any, stream grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		name, contract, ok := s.service(info.FullMethod)
		if !ok || (contract == nil && !s.exemptions[name]) {
			return status.Error(codes.FailedPrecondition, "unprotected service registration")
		}
		if contract == nil {
			return handler(srv, stream)
		}
		return contract.Stream()(srv, stream, info, handler)
	}
}

// ValidateRegisteredServices compares the actual gRPC server registrations to
// the strict contract table. Call once after all services are registered.
func (s *StrictServer) ValidateRegisteredServices(server *grpc.Server) error {
	registered := server.GetServiceInfo()
	var missing []string
	for name := range registered {
		if s.contracts[name] == nil && !s.exemptions[name] {
			missing = append(missing, name)
		}
	}
	if len(missing) != 0 {
		return fmt.Errorf("contract service coverage incomplete: unprotected=%s", strings.Join(missing, ","))
	}
	return nil
}
