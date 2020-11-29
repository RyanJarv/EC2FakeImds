package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
)

func handleRequest(ctx context.Context, event events.CloudWatchEvent) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Println("[ERROR]", r)
			return
		}
	}()

	m, err := event.Detail.MarshalJSON()
	if err != nil {
		panic(err)
	}

	var runEvent RunInstancesEvent
	if err := json.Unmarshal(m, &runEvent); err != nil {
		panic(err)
	}

	marshal, err := json.Marshal(runEvent)
	if err != nil {
		panic(err)
	}
	fmt.Printf("runEvent: %s\n", marshal)
}

func main() {
	lambda.Start(handleRequest)
}
