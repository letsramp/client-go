package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"

	"github.com/gin-gonic/gin"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"

	pb "skyramp.dev/demo/echo/pb"
)

var (
	grpcPort = flag.Int("grpc-port", 12345, "grpc server port")
	restPort = flag.Int("rest-port", 12346, "rest server port")
)

type server struct {
	pb.UnimplementedEchoServer
}

func (s *server) Echo(ctx context.Context, in *pb.EchoRequest) (*pb.EchoReply, error) {
	return &pb.EchoReply{Message: fmt.Sprintf("%s pong", in.GetMessage())}, nil
}

func runGrpcServer(port int) error {
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		log.Printf("failed to listen: %v", err)
		return err
	}

	s := grpc.NewServer()

	pb.RegisterEchoServer(s, &server{})
	if err := s.Serve(lis); err != nil {
		fmt.Printf("failed to serve: %v", err)
		return err
	}

	return nil
}

type echoMessage struct {
	Message string `json:"message"`
}

var UnsupportedContentType = "unsupported content type"

func echoPost(c *gin.Context) {
	var message echoMessage

	if c.Request.ContentLength > 0 {
		contentType := c.Request.Header.Get("Content-Type")
		switch contentType {
		case "application/json", "":
			if err := c.BindJSON(&message); err != nil {
				log.Printf("bind failed for POST %s: %v\n", c.FullPath(), err)
				return
			}
		default:
			c.JSON(http.StatusBadRequest, gin.H{
				"error": UnsupportedContentType,
			})
			return
		}
	}

	c.JSON(http.StatusOK, echoMessage{
		Message: fmt.Sprintf("%s pong", message.Message),
	})
}

func echoGet(c *gin.Context) {
	param := c.Param("msg")

	c.JSON(http.StatusOK, echoMessage{
		Message: fmt.Sprintf("%s pong", param),
	})
}

func runRestServer(port int) error {
	router := gin.New()
	router.POST("/echo", echoPost)
	router.GET("/echo/:msg", echoGet)

	router.Run(fmt.Sprintf(":%d", *restPort))

	return nil
}

func main() {
	flag.Parse()

	g, _ := errgroup.WithContext(context.Background())

	g.Go(func() error {
		return runGrpcServer(*grpcPort)
	})
	g.Go(func() error {
		return runRestServer(*restPort)
	})

	if err := g.Wait(); err != nil {
		log.Fatalf("failed to run servers: %v\n", err)
	}
}
