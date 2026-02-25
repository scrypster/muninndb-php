package grpc

import (
	"context"

	googlegrpc "google.golang.org/grpc"
)

// TestableAuthUnaryInterceptor exposes the unexported authUnaryInterceptor
// for use by external tests in package grpc_test.
func (s *Server) TestableAuthUnaryInterceptor(ctx context.Context, req any, info *googlegrpc.UnaryServerInfo, handler googlegrpc.UnaryHandler) (any, error) {
	return s.authUnaryInterceptor(ctx, req, info, handler)
}

// TestableAuthStreamInterceptor exposes the unexported authStreamInterceptor
// for use by external tests in package grpc_test.
func (s *Server) TestableAuthStreamInterceptor(srv any, ss googlegrpc.ServerStream, info *googlegrpc.StreamServerInfo, handler googlegrpc.StreamHandler) error {
	return s.authStreamInterceptor(srv, ss, info, handler)
}
