package main

import (
	"context"
	"log"
	"net"

	pb "github.com/dunkymole/proto-contract/examples/servers/go/gen/demo/v1"
	contract "github.com/dunkymole/proto-contract/runtimes/go/protocontract"
	"google.golang.org/grpc"
)

type server struct {
	pb.UnimplementedEchoServiceServer
}

func (server) Echo(_ context.Context, req *pb.EchoRequest) (*pb.EchoResponse, error) {
	return &pb.EchoResponse{Text: req.Text, ServerLanguage: "Go"}, nil
}

func main() {
	lis, err := net.Listen("tcp", ":50051")
	if err != nil {
		log.Fatal(err)
	}
	s := grpc.NewServer(grpc.UnaryInterceptor(contract.UnaryServer("demo.echo", "1.1.0")))
	pb.RegisterEchoServiceServer(s, server{})
	log.Println("Go server listening on :50051")
	log.Fatal(s.Serve(lis))
}
