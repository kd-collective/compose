package server

import (
	"context"
	"errors"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	"github.com/docker/api/client"
	"github.com/docker/api/config"
	apicontext "github.com/docker/api/context"
	"github.com/docker/api/context/store"
	"github.com/docker/api/server/proxy"
)

// key is the key where the current docker context is stored in the metadata
// of a gRPC request
const key = "context_key"

// unaryServerInterceptor configures the context and sends it to the next handler
func unaryServerInterceptor(clictx context.Context) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		currentContext, err := getIncomingContext(ctx)
		if err != nil {
			currentContext, err = getConfigContext(clictx)
			if err != nil {
				return nil, err
			}
		}
		configuredCtx, err := configureContext(clictx, currentContext, info.FullMethod)
		if err != nil {
			return nil, err
		}

		return handler(configuredCtx, req)
	}
}

// streamServerInterceptor configures the context and sends it to the next handler
func streamServerInterceptor(clictx context.Context) grpc.StreamServerInterceptor {
	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		currentContext, err := getIncomingContext(ss.Context())
		if err != nil {
			currentContext, err = getConfigContext(clictx)
			if err != nil {
				return err
			}
		}
		ctx, err := configureContext(clictx, currentContext, info.FullMethod)
		if err != nil {
			return err
		}

		return handler(srv, &contextServerStream{
			ss:  ss,
			ctx: ctx,
		})
	}
}

// Returns the current context from the configuration file
func getConfigContext(ctx context.Context) (string, error) {
	configDir := config.Dir(ctx)
	configFile, err := config.LoadFile(configDir)
	if err != nil {
		return "", err
	}
	return configFile.CurrentContext, nil
}

// Returns the context set by the caller if any, error otherwise
func getIncomingContext(ctx context.Context) (string, error) {
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		if key, ok := md[key]; ok {
			return key[0], nil
		}
	}

	return "", errors.New("not found")
}

// configureContext populates the request context with objects the client
// needs: the context store and the api client
func configureContext(ctx context.Context, currentContext string, method string) (context.Context, error) {
	configDir := config.Dir(ctx)

	ctx = apicontext.WithCurrentContext(ctx, currentContext)

	// The contexts service doesn't need the client
	if !strings.Contains(method, "/com.docker.api.protos.context.v1.Contexts") {
		c, err := client.New(ctx)
		if err != nil {
			return nil, err
		}

		ctx, err = proxy.WithClient(ctx, c)
		if err != nil {
			return nil, err
		}
	}

	s, err := store.New(store.WithRoot(configDir))
	if err != nil {
		return nil, err
	}
	ctx = store.WithContextStore(ctx, s)

	return ctx, nil
}
