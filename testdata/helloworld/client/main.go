/*
 *
 * Copyright 2015 gRPC authors.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 */

// Package main implements a client for Greeter service.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"runtime"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	pb "google.golang.org/grpc/examples/helloworld"
)

const (
	defaultName = "world"
)

var (
	addr     = flag.String("addr", "localhost:50051", "the address to connect to")
	name     = flag.String("name", defaultName, "Name to greet")
	conns    = flag.Int("conn", 1, "number of connections")
	duration = flag.Int("duration", 10, "duration of test")
	cpus     = flag.Int("cpus", 1, "number of cpus")
	wait     = flag.Int("wait", 10, "seconds to wait before start connection")
)

var lock sync.RWMutex

var (
	request     = 0
	lastRequest = 0
)

func monitorFunc(ctx context.Context) error {
	c := time.Tick(time.Second)
	for _ = range c {
		copyProcessed := 0
		copyLastProcessed := 0
		lock.RLock()
		copyProcessed = request
		copyLastProcessed = lastRequest
		lastRequest = request
		lock.RUnlock()

		newProcessed := copyProcessed - copyLastProcessed
		log.Printf("RPS = %d", newProcessed)

		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil
		}
	}

	return nil
}

func do(ctx context.Context) error {
	// Set up a connection to the server.
	conn, err := grpc.Dial(*addr, grpc.WithInsecure())
	if err != nil {
		log.Fatalf("did not connect: %v", err)
	}
	defer conn.Close()
	c := pb.NewGreeterClient(conn)

	tick := time.Tick(100 * time.Millisecond)

	cnt := 0
	for {
		_, err = c.SayHello(ctx, &pb.HelloRequest{Name: *name})
		if err != nil {
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return nil
			}

			log.Fatalf("could not greet: %v", err)
		}
		cnt++

		select {
		case <-tick:
			lock.Lock()
			request += cnt
			lock.Unlock()
			cnt = 0
		default:
		}
	}

	return nil
}

func main() {
	flag.Parse()
	runtime.GOMAXPROCS(*cpus)

	if *wait != 0 {
		time.Sleep(time.Duration(*wait) * time.Second)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(*duration)*time.Second)
	defer cancel()
	g, gCtx := errgroup.WithContext(ctx)

	for i := 0; i < *conns; i++ {
		g.Go(func() error {
			return do(gCtx)
		})
	}

	g.Go(func() error { return monitorFunc(gCtx) })

	if err := g.Wait(); err != nil {
		fmt.Printf("something wrong: %v", err)
	}

	// summarize RPS
	fmt.Printf("rps = %d requests = %d\n", request / *duration, request)

	time.Sleep(100000 * time.Second)
}
