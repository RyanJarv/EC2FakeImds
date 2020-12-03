package main

import (
	"encoding/json"
	"fmt"
	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"os"
)

func GetRequiredEnv(name string) string {
	if v := os.Getenv(name); v == "" {
		panic(fmt.Errorf("required environment variable '%s' not found", name))
	} else {
		return v
	}
}

func UnMarshallEvent(event events.CloudWatchEvent) (RunInstancesEvent) {
	m, err := event.Detail.MarshalJSON()
	if err != nil {
		panic(err)
	}

	var runEvent RunInstancesEvent
	if err := json.Unmarshal(m, &runEvent); err != nil {
		panic(err)
	}
	return runEvent
}

// GetAssociationId returns info about an association between a subnet and a table.
func GetAssociationId(table *types.RouteTable, subnetId *string) *types.RouteTableAssociation {
	for _, assoc := range table.Associations {
		if assoc.SubnetId != nil && *assoc.SubnetId == *subnetId {
			return assoc
		}
	}
	return nil
}

