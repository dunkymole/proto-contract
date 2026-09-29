package main

import (
	"context"
	"io"
	"log"
	"net"
	"strings"

	echocontract "github.com/dunkymole/proto-contract/gen/go/contracts/echo"
	pb "github.com/dunkymole/proto-contract/gen/go/demo/v1"
	"google.golang.org/grpc"
)

type server struct {
	pb.UnimplementedEchoServiceServer
}

func (server) Echo(_ context.Context, req *pb.EchoRequest) (*pb.EchoResponse, error) {
	return &pb.EchoResponse{Text: req.Text, ServerLanguage: "Go"}, nil
}

func (server) EchoClientStream(stream pb.EchoService_EchoClientStreamServer) error {
	var values []string
	for {
		req, err := stream.Recv()
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
		values = append(values, req.Text)
	}
	return stream.SendAndClose(&pb.EchoResponse{Text: strings.Join(values, ","), ServerLanguage: "Go"})
}
func (server) EchoServerStream(req *pb.EchoRequest, stream pb.EchoService_EchoServerStreamServer) error {
	return stream.Send(&pb.EchoResponse{Text: req.Text, ServerLanguage: "Go"})
}
func (server) EchoDuplex(stream pb.EchoService_EchoDuplexServer) error {
	for {
		req, err := stream.Recv()
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
		if err := stream.Send(&pb.EchoResponse{Text: req.Text, ServerLanguage: "Go"}); err != nil {
			return err
		}
	}
}

func main() {
	lis, err := net.Listen("tcp", ":50051")
	if err != nil {
		log.Fatal(err)
	}
	strict, err := echocontract.StrictServer()
	if err != nil {
		log.Fatal(err)
	}
	s := grpc.NewServer(grpc.ChainUnaryInterceptor(strict.Unary()), grpc.ChainStreamInterceptor(strict.Stream()))
	pb.RegisterEchoServiceServer(s, server{})
	if err := strict.ValidateRegisteredServices(s); err != nil {
		log.Fatal(err)
	}
	log.Println("Go server listening on :50051")
	log.Fatal(s.Serve(lis))
}
