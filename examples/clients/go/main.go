package main

import (
	"context"
	"fmt"
	"log"
	"time"

	pb "github.com/dunkymole/proto-contract/gen/go/demo/v1"
	contract "github.com/dunkymole/proto-contract/runtimes/go/protocontract"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

var servers = map[string]string{"Go": "go-server:50051", "Java": "java-server:50053", ".NET": "dotnet-server:50054", "Python": "python-server:50055"}

func call(target, version string) (*pb.EchoResponse, error) {
	conn, err := grpc.NewClient(target, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithUnaryInterceptor(contract.UnaryClient("demo.echo", version)))
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	return pb.NewEchoServiceClient(conn).Echo(ctx, &pb.EchoRequest{Text: "hello", RequestId: "demo"})
}

func main() {
	for expected, target := range servers {
		response, err := call(target, "1.0.0")
		if err != nil {
			log.Fatal(err)
		}
		if response.Text != "hello" || response.ServerLanguage != expected {
			log.Fatalf("unexpected response from %s: %v", expected, response)
		}
		for _, version := range []string{"1.2.0", "2.0.0"} {
			_, err = call(target, version)
			if status.Code(err) != codes.FailedPrecondition {
				log.Fatalf("%s accepted %s: %v", expected, version, err)
			}
		}
		fmt.Printf("PASS Go client -> %s server\n", expected)
	}
	fmt.Println("Go client matrix passed")
}
