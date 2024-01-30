package main

import (
	"context"
	"flag"
	"fmt"

	"google.golang.org/grpc"

	pb "skyramp.dev/demo/echo/pb"
)

const (
	defaultMessage = "ping"
)

var (
	addr    = flag.String("addr", "localhost:12345", "the address to connect to")
	message = flag.String("message", defaultMessage, "message to ping")
)

func main() {
	flag.Parse()

	conn, err := grpc.Dial(*addr, grpc.WithInsecure())
	if err != nil {
		fmt.Printf("could not connect: %v\n", err)
		return
	}
	defer conn.Close()

	c := pb.NewEchoClient(conn)

	reply, err := c.Echo(context.Background(), &pb.EchoRequest{Message: *message})
	fmt.Printf("Response from server: %s\n", reply.GetMessage())
}
