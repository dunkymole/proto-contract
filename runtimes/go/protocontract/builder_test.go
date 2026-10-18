package protocontract_test

import (
	"context"
	"io"
	"net"
	"sync/atomic"
	"testing"
	"time"

	echo "github.com/dunkymole/proto-contract/gen/go/contracts/echo"
	pb "github.com/dunkymole/proto-contract/gen/go/demo/v1"
	"github.com/dunkymole/proto-contract/runtimes/go/protocontract"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

func TestCombinedRegistrationValidation(t *testing.T) {
	b := protocontract.NewServerBuilder()
	implementation := &streamProbe{}
	if err := echo.RegisterServerWithCallback(b, func(r grpc.ServiceRegistrar) { pb.RegisterUnprotectedServiceServer(r, &unprotectedProbe{}) }); err == nil {
		t.Fatal("accepted mismatched registration")
	}
	if err := echo.RegisterServer(b, implementation); err != nil {
		t.Fatal(err)
	}
	if err := echo.RegisterServer(b, implementation); err == nil {
		t.Fatal("accepted duplicate")
	}
	s, err := b.Build()
	if err != nil {
		t.Fatal(err)
	}
	defer s.Stop()
	if _, err := b.Build(); err == nil {
		t.Fatal("accepted second build")
	}
	if err := echo.RegisterServer(b, implementation); err == nil {
		t.Fatal("accepted mutation after build")
	}
	exempt := protocontract.NewServerBuilder(echo.Service)
	if err := echo.RegisterServer(exempt, implementation); err == nil {
		t.Fatal("accepted protected exemption")
	}
}

func TestStrictGuardPrecedesShortCircuitingApplicationChains(t *testing.T) {
	b := protocontract.NewServerBuilder()
	if err := echo.RegisterServer(b, &streamProbe{}); err != nil {
		t.Fatal(err)
	}
	var unaryCalls, streamCalls atomic.Int32
	server, err := b.Build(
		grpc.ChainUnaryInterceptor(func(context.Context, any, *grpc.UnaryServerInfo, grpc.UnaryHandler) (any, error) {
			unaryCalls.Add(1)
			return &pb.EchoResponse{Text: "cached"}, nil
		}),
		grpc.ChainStreamInterceptor(func(any, grpc.ServerStream, *grpc.StreamServerInfo, grpc.StreamHandler) error {
			streamCalls.Add(1)
			return nil
		}),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Stop()
	// A raw registration must stay closed even when an app interceptor skips handlers.
	pb.RegisterUnprotectedServiceServer(server, &unprotectedProbe{})
	listener := bufconn.Listen(1 << 20)
	go func() { _ = server.Serve(listener) }()
	conn, err := grpc.NewClient("passthrough:///chain", grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := pb.NewUnprotectedServiceClient(conn).Call(ctx, &pb.EchoRequest{}); status.Code(err) != codes.FailedPrecondition {
		t.Errorf("unprotected service bypassed guard: %v", err)
	}
	if _, err := pb.NewEchoServiceClient(conn).Echo(ctx, &pb.EchoRequest{}); status.Code(err) != codes.FailedPrecondition {
		t.Errorf("missing unary contract bypassed guard: %v", err)
	}
	stream, err := pb.NewEchoServiceClient(conn).EchoServerStream(ctx, &pb.EchoRequest{})
	if err == nil {
		_, err = stream.Recv()
	}
	if status.Code(err) != codes.FailedPrecondition {
		t.Errorf("missing stream contract bypassed guard: %v", err)
	}
	if unaryCalls.Load() != 0 || streamCalls.Load() != 0 {
		t.Errorf("rejected calls reached application interceptors: unary=%d stream=%d", unaryCalls.Load(), streamCalls.Load())
	}
	unaryCalls.Store(0)
	streamCalls.Store(0)
	good := echo.NewClient(conn)
	response, err := good.Echo(ctx, &pb.EchoRequest{})
	if err != nil || response.Text != "cached" || unaryCalls.Load() != 1 {
		t.Fatalf("valid unary did not reach app interceptor: %v %v", response, err)
	}
	accepted, err := good.EchoServerStream(ctx, &pb.EchoRequest{})
	if err == nil {
		_, err = accepted.Recv()
	}
	if err != io.EOF || streamCalls.Load() != 1 {
		t.Fatalf("valid stream did not reach app interceptor: %v", err)
	}
}

func TestBuilderRejectsReplacingStrictInterceptorSlots(t *testing.T) {
	for name, option := range map[string]grpc.ServerOption{
		"unary":  grpc.UnaryInterceptor(func(context.Context, any, *grpc.UnaryServerInfo, grpc.UnaryHandler) (any, error) { return nil, nil }),
		"stream": grpc.StreamInterceptor(func(any, grpc.ServerStream, *grpc.StreamServerInfo, grpc.StreamHandler) error { return nil }),
	} {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatal("allowed replacement of the strict interceptor slot")
				}
			}()
			server, _ := protocontract.NewServerBuilder().Build(option)
			if server != nil {
				server.Stop()
			}
		})
	}
}

func TestMultipleContractsShareConnection(t *testing.T) {
	b := protocontract.NewServerBuilder()
	if err := echo.RegisterServer(b, &streamProbe{}); err != nil {
		t.Fatal(err)
	}
	second, err := protocontract.NewServer("other.api", "3.0.0", "demo.v1.UnprotectedService")
	if err != nil {
		t.Fatal(err)
	}
	if err := b.Register(second, func(r grpc.ServiceRegistrar) { pb.RegisterUnprotectedServiceServer(r, &unprotectedProbe{}) }); err != nil {
		t.Fatal(err)
	}
	server, err := b.Build()
	if err != nil {
		t.Fatal(err)
	}
	defer server.Stop()
	listener := bufconn.Listen(1 << 20)
	go func() { _ = server.Serve(listener) }()
	conn, err := grpc.NewClient("passthrough:///shared", grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	first := echo.NewClient(conn)
	other, err := protocontract.NewClient("other.api", "3.0.0", "demo.v1.UnprotectedService")
	if err != nil {
		t.Fatal(err)
	}
	secondClient := pb.NewUnprotectedServiceClient(other.Bind(conn))
	for i := 0; i < 3; i++ {
		if _, err := first.Echo(context.Background(), &pb.EchoRequest{}); err != nil {
			t.Fatal(err)
		}
		if _, err := secondClient.Call(context.Background(), &pb.EchoRequest{}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := pb.NewEchoServiceClient(conn).Echo(context.Background(), &pb.EchoRequest{}); err == nil {
		t.Fatal("helper modified shared connection")
	}
}
