package connectutil

import (
	"fmt"

	"connectrpc.com/connect/v2"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
)

// ReceiveFirstMessage reads the first request message of the RPC from stream
// into a newly allocated message of the request type described by spec.
//
// In connect-go v2 server interceptors run before the generated handler decodes
// the request, so interceptors that need the decoded request message (for
// example to authorize a project or tenant scoped method) use this helper and
// then hand the stream produced by [ReplayStream] to the next interceptor.
//
// ok is false when the method carries no protobuf input descriptor, in which
// case the caller should invoke the next interceptor with the original stream.
func ReceiveFirstMessage(stream connect.ServerStream, spec connect.Spec) (msg proto.Message, ok bool, err error) {
	md, ok := spec.Schema.(protoreflect.MethodDescriptor)
	if !ok || md == nil || md.Input() == nil {
		return nil, false, nil
	}

	mt, err := protoregistry.GlobalTypes.FindMessageByName(md.Input().FullName())
	if err != nil {
		return nil, false, fmt.Errorf("unable to find message type %q: %w", md.Input().FullName(), err)
	}

	msg = mt.New().Interface()
	if err := stream.Receive(msg); err != nil {
		return nil, false, err
	}

	return msg, true, nil
}

// ReplayStream wraps stream so that the buffered first request message is
// returned by the first Receive call and all subsequent calls are delegated to
// the underlying stream.
func ReplayStream(stream connect.ServerStream, first proto.Message) connect.ServerStream {
	return &replayStream{ServerStream: stream, first: first}
}

type replayStream struct {
	connect.ServerStream
	first    proto.Message
	replayed bool
}

func (s *replayStream) Receive(msg any) error {
	if !s.replayed {
		s.replayed = true
		if dst, ok := msg.(proto.Message); ok && s.first != nil {
			proto.Reset(dst)
			proto.Merge(dst, s.first)
			return nil
		}
	}

	return s.ServerStream.Receive(msg)
}
