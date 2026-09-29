package protocontract_test

import (
	"context"
	"net"
	"sync/atomic"
	"testing"

	demov1 "github.com/dunkymole/proto-contract/gen/go/demo/v1"
	"github.com/dunkymole/proto-contract/runtimes/go/protocontract"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

type streamProbe struct {
	demov1.UnimplementedEchoServiceServer
	called atomic.Int32
}

type unprotectedProbe struct {
	demov1.UnimplementedUnprotectedServiceServer
	called atomic.Int32
}

func (s *unprotectedProbe) Call(_ context.Context, req *demov1.EchoRequest) (*demov1.EchoResponse, error) {
	s.called.Add(1)
	return &demov1.EchoResponse{Text: req.GetText()}, nil
}

func (s *streamProbe) Echo(_ context.Context, req *demov1.EchoRequest) (*demov1.EchoResponse, error) {
	s.called.Add(1)
	return &demov1.EchoResponse{Text: req.GetText()}, nil
}
func (s *streamProbe) EchoClientStream(stream grpc.ClientStreamingServer[demov1.EchoRequest, demov1.EchoResponse]) error {
	s.called.Add(1)
	var text string
	for { req, err := stream.Recv(); if err != nil { break }; text = req.GetText() }
	return stream.SendAndClose(&demov1.EchoResponse{Text: text})
}
func (s *streamProbe) EchoServerStream(req *demov1.EchoRequest, stream grpc.ServerStreamingServer[demov1.EchoResponse]) error {
	s.called.Add(1)
	return stream.Send(&demov1.EchoResponse{Text: req.GetText()})
}
func (s *streamProbe) EchoDuplex(stream grpc.BidiStreamingServer[demov1.EchoRequest, demov1.EchoResponse]) error {
	s.called.Add(1)
	for { req, err := stream.Recv(); if err != nil { return nil }; if err := stream.Send(&demov1.EchoResponse{Text: req.GetText()}); err != nil { return err } }
}

func TestRealStreamingContractEnforcement(t *testing.T) {
	strictServer, err := protocontract.NewServer("demo.echo", "1.1.0", "demo.v1.EchoService")
	if err != nil { t.Fatal(err) }
	registry, err := protocontract.NewStrictServer(map[string]*protocontract.Server{"demo.v1.EchoService": strictServer}, nil)
	if err != nil { t.Fatal(err) }
	listener := bufconn.Listen(1 << 20)
	server := grpc.NewServer(grpc.UnaryInterceptor(registry.Unary()), grpc.StreamInterceptor(registry.Stream()))
	probe := &streamProbe{}
	unprotected := &unprotectedProbe{}
	demov1.RegisterEchoServiceServer(server, probe)
	demov1.RegisterUnprotectedServiceServer(server, unprotected)
	if err := registry.ValidateRegisteredServices(server); err == nil {
		t.Fatal("startup registration validation accepted an unprotected service")
	}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)
	client, err := protocontract.NewClient("demo.echo", "1.1.0", "demo.v1.EchoService")
	if err != nil { t.Fatal(err) }
	dialOptions := []grpc.DialOption{grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }), grpc.WithTransportCredentials(insecure.NewCredentials())}
	conn, err := grpc.NewClient("passthrough:///bufconn", append(dialOptions, grpc.WithUnaryInterceptor(client.Unary()), grpc.WithStreamInterceptor(client.Stream()))...)
	if err != nil { t.Fatal(err) }
	t.Cleanup(func() { _ = conn.Close() })
	accepted := demov1.NewEchoServiceClient(conn)
	req := &demov1.EchoRequest{Text: "stream"}
	if _, err := accepted.Echo(context.Background(), req); err != nil { t.Fatal(err) }
	ss, err := accepted.EchoServerStream(context.Background(), req); if err != nil { t.Fatal(err) }; if _, err := ss.Recv(); err != nil { t.Fatal(err) }
	cs, err := accepted.EchoClientStream(context.Background()); if err != nil { t.Fatal(err) }; if err := cs.Send(req); err != nil { t.Fatal(err) }; if _, err := cs.CloseAndRecv(); err != nil { t.Fatal(err) }
	duplex, err := accepted.EchoDuplex(context.Background()); if err != nil { t.Fatal(err) }; if err := duplex.Send(req); err != nil { t.Fatal(err) }; if _, err := duplex.Recv(); err != nil { t.Fatal(err) }; _ = duplex.CloseSend()
	if got := probe.called.Load(); got != 4 { t.Fatalf("accepted call handlers = %d, want 4", got) }

	rawConn, err := grpc.NewClient("passthrough:///bufconn", dialOptions...)
	if err != nil { t.Fatal(err) }
	t.Cleanup(func() { _ = rawConn.Close() })
	raw := demov1.NewEchoServiceClient(rawConn)
	unknown := demov1.NewUnprotectedServiceClient(rawConn)
	ctxValid := metadata.NewOutgoingContext(context.Background(), metadata.Pairs(protocontract.MetadataKey, "demo.echo@1.1.0"))
	if _, err := unknown.Call(ctxValid, &demov1.EchoRequest{Text: "must-not-run"}); status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("unmapped service error = %v, want FailedPrecondition", err)
	}
	if got := unprotected.called.Load(); got != 0 { t.Fatalf("unmapped handler calls = %d, want 0", got) }
	for _, tc := range []struct { name, value string }{{"missing", ""}, {"wrong", "demo.echo@2.0.0"}} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			if tc.value != "" { ctx = metadata.NewOutgoingContext(ctx, metadata.Pairs(protocontract.MetadataKey, tc.value)) }
			assertRejected := func(err error) { t.Helper(); if status.Code(err) != codes.FailedPrecondition { t.Fatalf("error = %v, want FailedPrecondition", err) } }
			serverStream, err := raw.EchoServerStream(ctx, req); if err != nil { assertRejected(err) } else { _, err = serverStream.Recv(); assertRejected(err) }
			clientStream, err := raw.EchoClientStream(ctx); if err != nil { assertRejected(err) } else { _ = clientStream.Send(req); _, err = clientStream.CloseAndRecv(); assertRejected(err) }
			bidi, err := raw.EchoDuplex(ctx); if err != nil { assertRejected(err) } else { _ = bidi.Send(req); _, err = bidi.Recv(); assertRejected(err) }
			if got := probe.called.Load(); got != 4 { t.Fatalf("rejected calls reached handler: count=%d, want 4", got) }
		})
	}
}

func TestExplicitServiceExemption(t *testing.T) {
	registry, err := protocontract.NewStrictServer(nil, []string{"demo.v1.UnprotectedService"})
	if err != nil { t.Fatal(err) }
	listener := bufconn.Listen(1 << 20)
	server := grpc.NewServer(grpc.UnaryInterceptor(registry.Unary()), grpc.StreamInterceptor(registry.Stream()))
	probe := &unprotectedProbe{}
	demov1.RegisterUnprotectedServiceServer(server, probe)
	if err := registry.ValidateRegisteredServices(server); err != nil { t.Fatal(err) }
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)
	conn, err := grpc.NewClient("passthrough:///exempt", grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil { t.Fatal(err) }
	t.Cleanup(func() { _ = conn.Close() })
	response, err := demov1.NewUnprotectedServiceClient(conn).Call(context.Background(), &demov1.EchoRequest{Text: "exempt"})
	if err != nil { t.Fatal(err) }
	if response.GetText() != "exempt" || probe.called.Load() != 1 { t.Fatalf("explicit exemption did not dispatch: response=%v calls=%d", response, probe.called.Load()) }
}
