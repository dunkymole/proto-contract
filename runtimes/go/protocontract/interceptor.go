package protocontract

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const MetadataKey = "x-proto-contract"

func UnaryClient(api, clientVersion string) grpc.UnaryClientInterceptor {
	value := api + "@" + clientVersion
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		ctx = metadata.AppendToOutgoingContext(ctx, MetadataKey, value)
		return invoker(ctx, method, req, reply, cc, opts...)
	}
}

func UnaryServer(api, serverVersion string) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		md, _ := metadata.FromIncomingContext(ctx)
		values := md.Get(MetadataKey)
		if len(values) != 1 {
			return nil, status.Error(codes.FailedPrecondition, "missing x-proto-contract metadata")
		}
		if err := Compatible(api, values[0], serverVersion); err != nil {
			return nil, status.Error(codes.FailedPrecondition, err.Error())
		}
		return handler(ctx, req)
	}
}

func Compatible(api, offered, serverVersion string) error {
	parts := strings.Split(offered, "@")
	if len(parts) != 2 || parts[0] != api {
		return fmt.Errorf("expected contract %s@MAJOR.MINOR.PATCH", api)
	}
	c, err := parse(parts[1])
	if err != nil {
		return err
	}
	s, err := parse(serverVersion)
	if err != nil {
		return err
	}
	if c[0] != s[0] || c[1] > s[1] {
		return fmt.Errorf("incompatible contract: client %s, server %s", parts[1], serverVersion)
	}
	return nil
}
func parse(v string) ([3]int, error) {
	var out [3]int
	p := strings.Split(v, ".")
	if len(p) != 3 {
		return out, fmt.Errorf("invalid semantic version %q", v)
	}
	for i := range p {
		n, e := strconv.Atoi(p[i])
		if e != nil || n < 0 {
			return out, fmt.Errorf("invalid semantic version %q", v)
		}
		out[i] = n
	}
	return out, nil
}
