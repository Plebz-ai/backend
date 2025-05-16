package grpc

import (
	"log"
	"net"

	"google.golang.org/grpc"
)

// TODO: Import generated proto and implement service
// import pb "ai-agent-character-demo/backend/conversation/grpc/proto"

func StartGRPCServer(port string) {
	lis, err := net.Listen("tcp", ":"+port)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	grpcServer := grpc.NewServer()
	// TODO: Register your conversation service here
	// pb.RegisterConversationServiceServer(grpcServer, &ConversationServer{})
	log.Printf("gRPC server listening on %s", port)
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
} 