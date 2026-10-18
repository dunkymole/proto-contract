package protocontract_test

import (
	"context"
	"net"
	"testing"

	echo "github.com/dunkymole/proto-contract/gen/go/contracts/echo"
	pb "github.com/dunkymole/proto-contract/gen/go/demo/v1"
	"github.com/dunkymole/proto-contract/runtimes/go/protocontract"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
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
