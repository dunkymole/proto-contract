package main

import (
	"context"
	"fmt"
	"log"
	"time"

	echocontract "github.com/dunkymole/proto-contract/gen/go/contracts/echo"
	pb "github.com/dunkymole/proto-contract/gen/go/demo/v1"
	contract "github.com/dunkymole/proto-contract/runtimes/go/protocontract"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

var servers = map[string]string{"Go": "go-server:50051", "Java": "java-server:50053", ".NET": "dotnet-server:50054", "Python": "python-server:50055"}

func call(target string, interceptor grpc.UnaryClientInterceptor) (*pb.EchoResponse, error) {
	conn, err := grpc.NewClient(target, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithUnaryInterceptor(interceptor), grpc.WithStreamInterceptor(echocontract.ClientStreamInterceptor()))
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	return pb.NewEchoServiceClient(conn).Echo(ctx, &pb.EchoRequest{Text: "hello", RequestId: "demo"})
}

func callStreams(target string) error {
	conn, err := grpc.NewClient(target, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithUnaryInterceptor(echocontract.ClientInterceptor()), grpc.WithStreamInterceptor(echocontract.ClientStreamInterceptor()))
	if err != nil {
		return err
	}
	defer conn.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	client := pb.NewEchoServiceClient(conn)
	request := &pb.EchoRequest{Text: "stream", RequestId: "demo"}
	clientStream, err := client.EchoClientStream(ctx)
	if err != nil {
		return err
	}
	if err = clientStream.Send(request); err != nil {
		return err
	}
	response, err := clientStream.CloseAndRecv()
	if err != nil {
		return err
	}
	if response.Text != "stream" {
		return fmt.Errorf("unexpected client-stream response %q", response.Text)
	}
	serverStream, err := client.EchoServerStream(ctx, request)
	if err != nil {
		return err
	}
	response, err = serverStream.Recv()
	if err != nil {
		return err
	}
	if response.Text != "stream" {
		return fmt.Errorf("unexpected server-stream response %q", response.Text)
	}
	bidi, err := client.EchoDuplex(ctx)
	if err != nil {
		return err
	}
	if err = bidi.Send(request); err != nil {
		return err
	}
	if err = bidi.CloseSend(); err != nil {
		return err
	}
	response, err = bidi.Recv()
	if err != nil {
		return err
	}
	if response.Text != "stream" {
		return fmt.Errorf("unexpected duplex response %q", response.Text)
	}
	return nil
}

func expectRejectedStream(target string, interceptor grpc.StreamClientInterceptor) error {
	conn, err := grpc.NewClient(target, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithStreamInterceptor(interceptor))
	if err != nil {
		return err
	}
	defer conn.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	stream, err := pb.NewEchoServiceClient(conn).EchoServerStream(ctx, &pb.EchoRequest{Text: "must-not-run"})
	if err != nil {
		if status.Code(err) == codes.FailedPrecondition {
			return nil
		}
		return err
	}
	_, err = stream.Recv()
	if status.Code(err) != codes.FailedPrecondition {
		return fmt.Errorf("expected FAILED_PRECONDITION, got %v", err)
	}
	return nil
}

func main() {
	for expected, target := range servers {
		response, err := call(target, echocontract.ClientInterceptor())
		if err != nil {
			log.Fatal(err)
		}
		if response.Text != "hello" || response.ServerLanguage != expected {
			log.Fatalf("unexpected response from %s: %v", expected, response)
		}
		if err := callStreams(target); err != nil {
			log.Fatalf("streaming call to %s failed: %v", expected, err)
		}
		if err := expectRejectedStream(target, contract.StreamClient("demo.echo", "1.2.0")); err != nil {
			log.Fatalf("%s accepted wrong streaming contract: %v", expected, err)
		}
		if err := expectRejectedStream(target, nil); err != nil {
			log.Fatalf("%s accepted missing streaming contract: %v", expected, err)
		}
		for _, version := range []string{"1.2.0", "2.0.0"} {
			_, err = call(target, contract.UnaryClient("demo.echo", version))
			if status.Code(err) != codes.FailedPrecondition {
				log.Fatalf("%s accepted %s: %v", expected, version, err)
			}
		}
		fmt.Printf("PASS Go client -> %s server\n", expected)
	}
	fmt.Println("Go client matrix passed")
}
