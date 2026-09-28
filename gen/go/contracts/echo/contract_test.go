package echocontract

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func TestGeneratedServerEnforcesItsLock(t *testing.T) {
	parts := strings.Split(Version, ".")
	major, _ := strconv.Atoi(parts[0])
	minor, _ := strconv.Atoi(parts[1])
	for _, tc := range []struct {
		name, offered string
		allowed       bool
	}{
		{"same lock", API + "@" + Version, true},
		{"patch ignored", fmt.Sprintf("%s@%d.%d.99", API, major, minor), true},
		{"newer minor", fmt.Sprintf("%s@%d.%d.0", API, major, minor+1), false},
		{"different major", fmt.Sprintf("%s@%d.0.0", API, major+1), false},
		{"wrong API", "wrong.api@" + Version, false},
		{"missing", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			if tc.offered != "" {
				ctx = metadata.NewIncomingContext(ctx, metadata.Pairs("x-proto-contract", tc.offered))
			}
			called := false
			_, err := ServerInterceptor()(ctx, nil, &grpc.UnaryServerInfo{FullMethod: "/" + Service + "/Echo"}, func(context.Context, any) (any, error) {
				called = true
				return nil, nil
			})
			if tc.allowed && err != nil {
				t.Fatal(err)
			}
			if !tc.allowed && status.Code(err) != codes.FailedPrecondition {
				t.Fatalf("expected rejection, got %v", err)
			}
			if called != tc.allowed {
				t.Fatal("unexpected handler execution")
			}
		})
	}
}

func TestGeneratedServerLeavesOtherServicesToTheirInterceptor(t *testing.T) {
	called := false
	_, err := ServerInterceptor()(context.Background(), nil, &grpc.UnaryServerInfo{FullMethod: "/other.Service/Call"}, func(context.Context, any) (any, error) {
		called = true
		return nil, nil
	})
	if err != nil || !called {
		t.Fatal("unrelated service was intercepted")
	}
}
