package main

import (
	"context"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/pkg/errors"
)

// CheckIfInUse returns true if this script has been used but and the route tables haven't been reverted yet
func CheckIfInUse(currentTable *types.RouteTable) bool {
	for _, tag := range currentTable.Tags {
		if *tag.Key == OrigRouteTableId {
			return true
		}
	}
	return false
}

func FirstInstanceByName(ctx context.Context, name *string) (subnet *types.Instance) {
	if instances, err := client.DescribeInstances(ctx, &ec2.DescribeInstancesInput{
		Filters: []*types.Filter{
			{
				Name: aws.String("tag:Name"),
				Values: []*string{name},
			},
		},
	}); err != nil {
		panic(errors.Wrapf(err, "finding instance with name: %s", *name))
	} else {
		return instances.Reservations[0].Instances[0]
	}
}

func GetTableByInstance(ctx context.Context, instance ResponseInstanceItems) *types.RouteTable {
	tables, err := client.DescribeRouteTables(ctx, &ec2.DescribeRouteTablesInput{
		Filters: []*types.Filter{
			{
				Name:   aws.String("association.subnet-id"),
				Values: []*string{aws.String(instance.SubnetId)},
			},
		},
	})
	if err != nil {
		panic(err)
	}

	// If no route tables returned from subnet search then it's using the vpc route table implicitly
	if len(tables.RouteTables) < 1 {
		return GetTableByVpc(ctx, instance.VpcId)
	} else {
		return tables.RouteTables[0]
	}
}

func GetTableByVpc(ctx context.Context, vpcId string) *types.RouteTable {
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
		panic(errors.Wrap(err, "describing route tables"))
	}

	if tables := tables.RouteTables; len(tables) != 1 {
		panic(fmt.Errorf("expected one table but got %d: %v", len(tables), tables))
	} else {
		return tables[0]
	}
}

