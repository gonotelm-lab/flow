package interceptor

import (
	"buf.build/go/protovalidate"
	protovalidatemid "github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/protovalidate"
	"google.golang.org/grpc"
)

var validator protovalidate.Validator

func init() {
	var err error
	validator, err = protovalidate.New(protovalidate.WithFailFast())
	if err != nil {
		panic(err)
	}
}

func validateUnaryInterceptor() grpc.UnaryServerInterceptor {
	return protovalidatemid.UnaryServerInterceptor(validator)
}

func validateStreamInterceptor() grpc.StreamServerInterceptor {
	return protovalidatemid.StreamServerInterceptor(validator)
}
