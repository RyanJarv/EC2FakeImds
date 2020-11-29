package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

var (
	client             *ec2.Client
	FakeImdsInstanceId string
	ActiveVpc          string
	ActiveSubnets      []string
)

func getRequiredEnv(name string) string {
	if v := os.Getenv(name); v == "" {
		panic(fmt.Errorf("required environment variable '%s' not found", name))
	} else {
		return v
	}
}

func setupEnv() {
	FakeImdsInstanceId = getRequiredEnv("FakeImdsInstanceId")
	ActiveVpc = getRequiredEnv("ActiveVpcs")
	ActiveSubnets = strings.Split(getRequiredEnv("ActiveSubnets"), ",")
}


func handleRequest(ctx context.Context, event events.CloudWatchEvent) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Println("[ERROR]", r)
			return
		}
	}()

	runEvent := UnMarshallEvent(event)

	var instances []ResponseInstanceItems
	for _, instance := range runEvent.ResponseElements.InstancesSet.Items {
		if instance.InstanceState.Name == "pending" {
			instances = append(instances, instance)
		}
	}

	toUpdate := ToUpdateSearch(instances)

	for vpc, subnet := range toUpdate {
		AttachFakeImdsRouting(ctx, vpc, subnet)
	}

	fmt.Printf("runEvent: %s\n", marshal)
}

func ToUpdateSearch(instances []ResponseInstanceItems) map[string]string {
	toUpdate := map[string]string{}
	for _, instance := range instances {
		if instance.VpcId != ActiveVpc {
			continue
		}

		active := false
		for _, activeSubnet := range ActiveSubnets {
			if instance.SubnetId == activeSubnet {
				active = true
			}
		}
		if !active {
			continue
		}

		if subnet, ok := toUpdate[instance.VpcId]; ok && instance.SubnetId == subnet {
			continue
		} else {
			toUpdate[instance.VpcId] = subnet
		}
	}
	return toUpdate
}

func AttachFakeImdsRouting(ctx context.Context, vpc string, subnet string) {
	oldTable := GetRouteTable(ctx, vpc, subnet)
	
	newTable := CopyRoutes(ctx, oldTable)
	CopyTags(ctx, oldTable, newTable)
	AddMetaTags(ctx, oldTable, newTable)
	AddFakeRoute(ctx, newTable)

	AttachTable(newTable)

}

func AddMetaTags(ctx context.Context, origTable *types.RouteTable, tmpTable *types.RouteTable) {
	client.CreateTags(ctx, &ec2.CreateTagsInput{
		Resources: []*string{dst.RouteTableId},
		Tags: []*types.Tag{
			{
				Key: aws.String("RouteTableId"),
				Value: origTable.RouteTableId,
			},
			{
				Key:   nil,
				Value: origTable.,
			},
		},
	})
}
	
func CopyTags(ctx context.Context, src, dst *types.RouteTable) {
	if tags, err := client.DescribeTags(ctx, &ec2.DescribeTagsInput{
		Filters:    []*types.Filter{
			{
				Name: aws.String("resource-type"),
				Values: aws.StringSlice([]string{"route-table"}),
			},
			{
				Name: aws.String("resource-id"),
				Values: aws.StringSlice([]string{*src.RouteTableId}),
			},
		},
	}); err != nil {
		panic(err)
	} else {
		var newTags []*types.Tag
		
		for _, tag := range tags.Tags {
			newTags = append(newTags, &types.Tag{
				Key:   tag.Key,
				Value: tag.Value,
			})
		}
		
		if _, err := client.CreateTags(ctx, &ec2.CreateTagsInput{
			Resources: []*string{dst.RouteTableId},
			Tags:      newTags,
		}); err != nil {
			panic(err)
		}
	}
}

func AttachTable(oldTable, newTable *types.RouteTable) {

}

func AddFakeRoute(ctx context.Context, table *types.RouteTable) *types.RouteTable {
	if _, err := client.CreateRoute(ctx, &ec2.CreateRouteInput{
		RouteTableId:         table.RouteTableId,
		DestinationCidrBlock: aws.String("169.254.169.254/32"),
		InstanceId:           aws.String(FakeImdsInstanceId),
	}); err != nil {
		panic(err)
	} else {
		return table
	}
}


func CopyRoutes(ctx context.Context, table *types.RouteTable) *types.RouteTable {
	newTable, err := client.CreateRouteTable(ctx, &ec2.CreateRouteTableInput{
		VpcId: table.VpcId,
	})
	if err != nil {
		panic(err)
	}

	for _, route := range table.Routes {
		if _, err := client.CreateRoute(ctx, &ec2.CreateRouteInput{
			RouteTableId:                newTable.RouteTable.RouteTableId,
			DestinationCidrBlock:        route.DestinationCidrBlock,
			DestinationIpv6CidrBlock:    route.DestinationIpv6CidrBlock,
			DestinationPrefixListId:     route.DestinationPrefixListId,
			EgressOnlyInternetGatewayId: route.EgressOnlyInternetGatewayId,
			GatewayId:                   route.GatewayId,
			InstanceId:                  route.InstanceId,
			LocalGatewayId:              route.LocalGatewayId,
			NatGatewayId:                route.NatGatewayId,
			NetworkInterfaceId:          route.NetworkInterfaceId,
			TransitGatewayId:            route.TransitGatewayId,
			VpcPeeringConnectionId:      route.VpcPeeringConnectionId,
		}); err != nil {
			panic(err)
		}
	}

	return newTable.RouteTable
}

		func GetRouteTable(ctx context.Context, vpc string, subnet string) *types.RouteTable {
	tables, err := client.DescribeRouteTables(ctx, &ec2.DescribeRouteTablesInput{
		Filters: []*types.Filter{
			{
				Name:   aws.String("association.subnet-id"),
				Values: []*string{aws.String(subnet)},
			},
		},
	})
	if err != nil {
		panic(err)
	}

	var table *types.RouteTable
	if l := len(tables.RouteTables); l != 1 {
		panic(fmt.Errorf("expected a single table associated with %s, but found %s. tables: %v", subnet, l, tables.RouteTables))
	} else {
		table = tables.RouteTables[0]
	}

	if *table.VpcId != vpc {
		panic(fmt.Errorf("expected subnet %s to be in vpc %s, but instead found vpc %s", subnet, vpc, table.VpcId))
	}

	return table
}

func CreateFakeImdsRoutTable(ctx context.Context, vpc string) *ec2.CreateRouteTableOutput {
	if table, err := client.CreateRouteTable(ctx, &ec2.CreateRouteTableInput{
		VpcId: aws.String(vpc),
	}); err != nil {
		panic(err)
	} else {
		return table
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

func main() {
	if conf, err := config.LoadDefaultConfig(); err != nil {
		panic(err)
	} else {
		client = ec2.NewFromConfig(conf)
	}
	lambda.Start(handleRequest)
}
