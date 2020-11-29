package main

type Attachment struct {
	AttachmentId string `json:"attachmentId"`
	DeviceIndex  int32  `json:"deviceIndex"`
}

type NetworkInterfaceSetItems struct {
	NetworkInterfaceId string     `json:"networkInterfaceId"`
	Attachment         Attachment `json:"attachment"`
}

type NetworkInterfaceSet struct {
	Items []NetworkInterfaceSetItems `json:"items"`
}

type InstanceState struct {
	Name string `json:"name"`
}

type Placement struct {
	AvailabilityZone string `json:"availabilityZone"`
}

type ResponseInstanceItems struct {
	InstanceId          string              `json:"instanceId"`
	InstanceState       InstanceState       `json:"instanceState"`
	ImageId             string              `json:"instanceId"`
	SubnetId            string              `json:"subnetId"`
	VpcId               string              `json:"vpcId"`
	PrivateIpAddress    string              `json:"privateIpAddress"`
	Placement           Placement           `json:"placement"`
	NetworkInterfaceSet NetworkInterfaceSet `json:"networkInterfaceSet"`
}

type ResponseInstanceSet struct {
	Items []ResponseInstanceItems `json:"items"`
}

type ResponseElements struct {
	InstancesSet ResponseInstanceSet `json:"instancesSet"`
}

type RunInstancesEvent struct {
	EventVersion      string            `json:"eventVersion"`
	EventName         string            `json:"eventName"`
	AwsRegion         string            `json:"awsRegion"`
	ResponseElements  ResponseElements  `json:"responseElements"`
}
