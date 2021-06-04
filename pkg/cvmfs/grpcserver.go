package cvmfs

import (
	"context"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"

	"github.com/cernops/cvmfs-csi/internal"
	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/kubernetes-csi/csi-lib-utils/protosanitizer"
	"github.com/rs/zerolog"
	"google.golang.org/grpc"
)

var requestCount = 0

type nonBlockingGRPCServer struct {
	wg      sync.WaitGroup
	server  *grpc.Server
	cleanup func()
}

func (s *nonBlockingGRPCServer) Start(endpoint string, ids csi.IdentityServer, cs csi.ControllerServer, ns csi.NodeServer) {
	s.wg.Add(1)
	go s.serve(endpoint, ids, cs, ns)
}

func (s *nonBlockingGRPCServer) Wait() {
	s.wg.Wait()
}

func (s *nonBlockingGRPCServer) Stop() {
	s.server.GracefulStop()
	s.cleanup()
}

func (s *nonBlockingGRPCServer) ForceStop() {
	s.server.Stop()
	s.cleanup()
}

func (s *nonBlockingGRPCServer) serve(ep string, ids csi.IdentityServer, cs csi.ControllerServer, ns csi.NodeServer) {
	log := internal.GetLogger("serve")
	listener, cleanup, err := listen(ep)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to open listener")
	}

	opts := []grpc.ServerOption{
		grpc.UnaryInterceptor(logGRPC),
	}
	server := grpc.NewServer(opts...)
	s.server = server
	s.cleanup = cleanup

	csi.RegisterIdentityServer(server, ids)
	csi.RegisterControllerServer(server, cs)
	csi.RegisterNodeServer(server, ns)

	log.Info().Str("address", listener.Addr().String()).Msg("Listening for connections")

	err = server.Serve(listener)
	log.Fatal().Err(err).Msg("server stopped")
}

func logGRPC(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	requestCount += 1
	log := internal.GetLogger("GRPC").With().Int("requestid", requestCount).Str("method", info.FullMethod).Logger()

	l := zerolog.InfoLevel
	l2 := zerolog.DebugLevel
	if info.FullMethod == "/csi.v1.Identity/Probe" {
		// This call occurs frequently, only log at trace level
		l = zerolog.TraceLevel
		l2 = zerolog.TraceLevel
	}

	log.WithLevel(l).Msg("GRPC request")
	log.WithLevel(l2).Msg(protosanitizer.StripSecrets(req).String())

	resp, err := handler(log.WithContext(ctx), req)

	if err != nil {
		log.Error().Err(err).Str("method", info.FullMethod).Msg("GRPC error")
	}

	return resp, err
}

func parseEndpoint(ep string) (string, string, error) {
	if strings.HasPrefix(strings.ToLower(ep), "unix://") || strings.HasPrefix(strings.ToLower(ep), "tcp://") {
		s := strings.SplitN(ep, "://", 2)
		if s[1] != "" {
			return s[0], s[1], nil
		}
		return "", "", fmt.Errorf("invalid endpoint: %v", ep)
	}
	// Assume everything else is a file path for a Unix Domain Socket.
	return "unix", ep, nil
}

func listen(endpoint string) (net.Listener, func(), error) {
	proto, addr, err := parseEndpoint(endpoint)
	if err != nil {
		return nil, nil, err
	}

	cleanup := func() {}
	if proto == "unix" {
		// addr = "/" + addr
		if err := os.Remove(addr); err != nil && !os.IsNotExist(err) { //nolint: vetshadow
			return nil, nil, fmt.Errorf("%s: %q", addr, err)
		}
		cleanup = func() {
			os.Remove(addr)
		}
	}

	log := internal.GetLogger("grpcServer")
	log.Info().Str("proto", proto).Str("addr", addr).Msg("opening listener")
	l, err := net.Listen(proto, addr)
	return l, cleanup, err
}
